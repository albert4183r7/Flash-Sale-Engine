# =============================================================================
# Flash Sale Engine - AWS Infrastructure (Singapore Region)
# =============================================================================

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Environment = var.environment
      Project     = "flashsale"
      ManagedBy   = "terraform"
    }
  }
}

locals {
  name   = "flashsale"
  region = var.region

  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 2)

  tags = {
    Environment = var.environment
    Project     = "flashsale"
  }
}

data "aws_availability_zones" "available" {}
data "aws_caller_identity" "current" {}

# =============================================================================
# VPC & NETWORKING
# =============================================================================

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = "${local.name}-vpc"
  cidr = local.vpc_cidr

  azs             = local.azs
  private_subnets = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 4, k)]
  public_subnets  = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 4, k + 2)]

  enable_nat_gateway   = true
  single_nat_gateway   = var.environment != "production"
  enable_dns_hostnames = true
  enable_dns_support   = true

  # Tags required for EKS
  public_subnet_tags = {
    "kubernetes.io/role/elb" = 1
  }
  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = 1
  }

  tags = local.tags
}

# =============================================================================
# EKS CLUSTER (with Fargate)
# =============================================================================

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "${local.name}-cluster"
  cluster_version = "1.29"

  cluster_endpoint_public_access = true

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  # Fargate profiles for serverless pods
  fargate_profiles = {
    flashsale = {
      name = "flashsale"
      selectors = [
        { namespace = "flashsale" }
      ]
    }
    kube_system = {
      name = "kube-system"
      selectors = [
        { namespace = "kube-system" }
      ]
    }
    monitoring = {
      name = "monitoring"
      selectors = [
        { namespace = "monitoring" }
      ]
    }
  }

  # Enable IRSA
  enable_irsa = true

  # Cluster addons
  cluster_addons = {
    coredns = {
      most_recent = true
      configuration_values = jsonencode({
        computeType = "Fargate"
      })
    }
    kube-proxy = {
      most_recent = true
    }
    vpc-cni = {
      most_recent = true
    }
  }

  tags = local.tags
}

# =============================================================================
# RDS POSTGRESQL (Multi-AZ)
# =============================================================================

resource "aws_db_subnet_group" "flashsale" {
  name       = "${local.name}-db-subnet"
  subnet_ids = module.vpc.private_subnets

  tags = {
    Name = "${local.name}-db-subnet"
  }
}

resource "aws_security_group" "rds" {
  name_prefix = "${local.name}-rds-"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [module.eks.cluster_security_group_id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.tags
}

resource "aws_db_instance" "main" {
  identifier = "${local.name}-db"

  engine         = "postgres"
  engine_version = "16"
  instance_class = var.db_instance_class

  allocated_storage     = 20
  max_allocated_storage = 100
  storage_type          = "gp3"
  storage_encrypted     = true

  db_name  = "flashsale"
  username = "flashsale"
  password = var.db_password

  multi_az               = var.environment == "production"
  db_subnet_group_name   = aws_db_subnet_group.flashsale.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  backup_retention_period = 0
  backup_window           = "03:00-04:00"
  maintenance_window      = "sun:04:00-sun:05:00"

  skip_final_snapshot = true  # Always skip for easy cleanup

  tags = local.tags
}

# =============================================================================
# ELASTICACHE REDIS
# =============================================================================

resource "aws_security_group" "redis" {
  name_prefix = "${local.name}-redis-"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [module.eks.cluster_security_group_id]
  }

  # Allow Fargate pods (they use ENIs in VPC CIDR, not cluster SG)
  ingress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr_block]
    description = "Allow Redis access from VPC (Fargate pods)"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.tags
}

resource "aws_elasticache_subnet_group" "flashsale" {
  name       = "${local.name}-redis-subnet"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id = "${local.name}-redis"
  description          = "Flash Sale Redis cluster"

  engine               = "redis"
  engine_version       = "7.0"
  node_type            = var.redis_node_type
  num_cache_clusters   = 2
  port                 = 6379
  parameter_group_name = "default.redis7"

  automatic_failover_enabled = true
  multi_az_enabled           = true

  subnet_group_name  = aws_elasticache_subnet_group.flashsale.name
  security_group_ids = [aws_security_group.redis.id]

  at_rest_encryption_enabled = true
  transit_encryption_enabled = false # Set true for production

  tags = local.tags
}

# =============================================================================
# SQS (Order Events Queue) - Serverless, cost-effective messaging
# =============================================================================

resource "aws_sqs_queue" "order_events" {
  name                       = "${local.name}-order-events"
  visibility_timeout_seconds = 60
  message_retention_seconds  = 86400 # 24 hours
  receive_wait_time_seconds  = 20    # Long polling

  tags = local.tags
}

resource "aws_sqs_queue" "order_events_dlq" {
  name                      = "${local.name}-order-events-dlq"
  message_retention_seconds = 1209600 # 14 days

  tags = local.tags
}

resource "aws_sqs_queue_redrive_policy" "order_events" {
  queue_url = aws_sqs_queue.order_events.id
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.order_events_dlq.arn
    maxReceiveCount     = 5
  })
}

# =============================================================================
# ECR REPOSITORIES
# =============================================================================

locals {
  services = [
    "api-gateway",
    "user-service",
    "product-service",
    "order-service",
    "purchase-service",
    "order-worker"
  ]
}

resource "aws_ecr_repository" "services" {
  for_each = toset(local.services)

  name                 = "${local.name}/${each.value}"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = local.tags
}

resource "aws_ecr_lifecycle_policy" "cleanup" {
  for_each   = aws_ecr_repository.services
  repository = each.value.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last 10 images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 10
      }
      action = {
        type = "expire"
      }
    }]
  })
}

# =============================================================================
# SECRETS MANAGER
# =============================================================================

resource "aws_secretsmanager_secret" "jwt_secret" {
  name                    = "${local.name}/jwt-secret"
  description             = "JWT signing secret for Flash Sale API"
  recovery_window_in_days = 0  # Allow immediate deletion for dev/test

  tags = local.tags
}

resource "aws_secretsmanager_secret_version" "jwt_secret" {
  secret_id     = aws_secretsmanager_secret.jwt_secret.id
  secret_string = var.jwt_secret
}

resource "aws_secretsmanager_secret" "db_password" {
  name                    = "${local.name}/db-password"
  description             = "RDS password for Flash Sale database"
  recovery_window_in_days = 0  # Allow immediate deletion for dev/test

  tags = local.tags
}

resource "aws_secretsmanager_secret_version" "db_password" {
  secret_id     = aws_secretsmanager_secret.db_password.id
  secret_string = var.db_password
}

# =============================================================================
# IAM ROLES FOR SERVICE ACCOUNTS (IRSA)
# =============================================================================

data "aws_iam_policy_document" "flashsale_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [module.eks.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${replace(module.eks.cluster_oidc_issuer_url, "https://", "")}:sub"
      values   = ["system:serviceaccount:flashsale:flashsale-sa"]
    }
  }
}

resource "aws_iam_role" "flashsale_workload" {
  name               = "${local.name}-workload-role"
  assume_role_policy = data.aws_iam_policy_document.flashsale_assume.json

  tags = local.tags
}

# SQS access policy
resource "aws_iam_role_policy" "sqs_access" {
  name = "${local.name}-sqs-access"
  role = aws_iam_role.flashsale_workload.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "sqs:SendMessage",
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes"
        ]
        Resource = [
          aws_sqs_queue.order_events.arn,
          aws_sqs_queue.order_events_dlq.arn
        ]
      }
    ]
  })
}

# Secrets Manager access policy
resource "aws_iam_role_policy" "secrets_access" {
  name = "${local.name}-secrets-access"
  role = aws_iam_role.flashsale_workload.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue"
        ]
        Resource = [
          aws_secretsmanager_secret.jwt_secret.arn,
          aws_secretsmanager_secret.db_password.arn
        ]
      }
    ]
  })
}

# =============================================================================
# AWS LOAD BALANCER CONTROLLER (for ALB Ingress)
# =============================================================================

# IAM policy for AWS Load Balancer Controller
data "http" "lbc_iam_policy" {
  url = "https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.7.1/docs/install/iam_policy.json"
}

resource "aws_iam_policy" "aws_load_balancer_controller" {
  name        = "${local.name}-AWSLoadBalancerControllerIAMPolicy"
  description = "IAM policy for AWS Load Balancer Controller"
  policy      = data.http.lbc_iam_policy.response_body
}

# IAM role for AWS Load Balancer Controller
data "aws_iam_policy_document" "aws_load_balancer_controller_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [module.eks.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${replace(module.eks.cluster_oidc_issuer_url, "https://", "")}:sub"
      values   = ["system:serviceaccount:kube-system:aws-load-balancer-controller"]
    }

    condition {
      test     = "StringEquals"
      variable = "${replace(module.eks.cluster_oidc_issuer_url, "https://", "")}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "aws_load_balancer_controller" {
  name               = "${local.name}-aws-load-balancer-controller"
  assume_role_policy = data.aws_iam_policy_document.aws_load_balancer_controller_assume.json

  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "aws_load_balancer_controller" {
  policy_arn = aws_iam_policy.aws_load_balancer_controller.arn
  role       = aws_iam_role.aws_load_balancer_controller.name
}

# Additional permissions for ALB Controller (not in default policy)
resource "aws_iam_role_policy" "aws_load_balancer_controller_extra" {
  name = "${local.name}-alb-controller-extra"
  role = aws_iam_role.aws_load_balancer_controller.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "elasticloadbalancing:DescribeListenerAttributes",
          "elasticloadbalancing:ModifyListenerAttributes"
        ]
        Resource = "*"
      }
    ]
  })
}

