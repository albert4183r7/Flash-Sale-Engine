# =============================================================================
# Flash Sale Engine - AWS Outputs
# =============================================================================

# EKS Cluster
output "eks_cluster_name" {
  description = "EKS cluster name"
  value       = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  description = "EKS cluster endpoint"
  value       = module.eks.cluster_endpoint
}

output "eks_cluster_certificate" {
  description = "EKS cluster CA certificate"
  value       = module.eks.cluster_certificate_authority_data
  sensitive   = true
}

# Configure kubectl command
output "configure_kubectl" {
  description = "Command to configure kubectl"
  value       = "aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.region}"
}

# RDS Database
output "rds_endpoint" {
  description = "RDS instance endpoint"
  value       = aws_db_instance.main.endpoint
}

output "rds_database_name" {
  description = "RDS database name"
  value       = aws_db_instance.main.db_name
}

# ElastiCache Redis
output "redis_endpoint" {
  description = "ElastiCache Redis primary endpoint"
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
}

output "redis_port" {
  description = "ElastiCache Redis port"
  value       = aws_elasticache_replication_group.redis.port
}

# SQS Queue
output "sqs_order_queue_url" {
  description = "SQS order events queue URL"
  value       = aws_sqs_queue.order_events.url
}

output "sqs_order_queue_arn" {
  description = "SQS order events queue ARN"
  value       = aws_sqs_queue.order_events.arn
}

# ECR Repositories
output "ecr_repositories" {
  description = "ECR repository URLs"
  value = {
    for k, v in aws_ecr_repository.services : k => v.repository_url
  }
}

# IAM Role for Workloads
output "workload_role_arn" {
  description = "IAM role ARN for Kubernetes workloads (IRSA)"
  value       = aws_iam_role.flashsale_workload.arn
}

# VPC
output "vpc_id" {
  description = "VPC ID"
  value       = module.vpc.vpc_id
}

output "private_subnets" {
  description = "Private subnet IDs"
  value       = module.vpc.private_subnets
}

# Connection strings for ConfigMap
output "postgres_connection_host" {
  description = "PostgreSQL host for K8s ConfigMap"
  value       = split(":", aws_db_instance.main.endpoint)[0]
}

# AWS Load Balancer Controller
output "aws_load_balancer_controller_role_arn" {
  description = "IAM role ARN for AWS Load Balancer Controller"
  value       = aws_iam_role.aws_load_balancer_controller.arn
}

