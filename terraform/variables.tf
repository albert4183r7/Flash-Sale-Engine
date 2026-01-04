# =============================================================================
# Flash Sale Engine - AWS Variables
# =============================================================================

variable "region" {
  description = "AWS region for deployment"
  type        = string
  default     = "ap-southeast-1" # Singapore - closest to Indonesia
}

variable "environment" {
  description = "Environment name (dev, staging, production)"
  type        = string
  default     = "production"
}

# Database Configuration
variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t3.micro" # Free tier eligible
}

variable "db_password" {
  description = "RDS master password"
  type        = string
  sensitive   = true
}

# Redis Configuration
variable "redis_node_type" {
  description = "ElastiCache node type"
  type        = string
  default     = "cache.t3.micro" # Free tier eligible
}

# Secrets
variable "jwt_secret" {
  description = "JWT signing secret"
  type        = string
  sensitive   = true
}
