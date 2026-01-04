# ⚡ Flash Sale Engine - AWS Deployment Guide

> **For Windows Users (PowerShell)** - All commands use PowerShell syntax.

---

## Prerequisites

| Tool | Check Command | Install |
|------|---------------|---------|
| AWS CLI v2 | `aws --version` | [Download](https://aws.amazon.com/cli/) |
| kubectl | `kubectl version --client` | `winget install Kubernetes.kubectl` |
| Terraform | `terraform --version` | `winget install Hashicorp.Terraform` |
| Docker Desktop | `docker --version` | [Download](https://www.docker.com/products/docker-desktop/) |
| Helm | `helm version` | `winget install Helm.Helm` |

---

## Part 1: Deploy Infrastructure

### Step 1: Create AWS IAM User

1. Go to [AWS Console](https://console.aws.amazon.com/) → **IAM** → **Users** → **Create user**
2. Name: `flashsale-deployer` → **Next**
3. Select **Attach policies directly** → **Create policy** → **JSON** tab
4. Paste this policy:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:*", "eks:*", "rds:*", "elasticache:*",
                "sqs:*", "ecr:*", "secretsmanager:*", "kms:*",
                "cloudwatch:*", "logs:*", "autoscaling:*",
                "elasticloadbalancing:*", "sts:GetCallerIdentity"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "iam:CreateRole", "iam:DeleteRole", "iam:AttachRolePolicy",
                "iam:DetachRolePolicy", "iam:PutRolePolicy", "iam:DeleteRolePolicy",
                "iam:GetRole", "iam:GetRolePolicy", "iam:ListRoles",
                "iam:ListRolePolicies", "iam:ListAttachedRolePolicies",
                "iam:CreateInstanceProfile", "iam:DeleteInstanceProfile",
                "iam:AddRoleToInstanceProfile", "iam:RemoveRoleFromInstanceProfile",
                "iam:GetInstanceProfile", "iam:ListInstanceProfiles",
                "iam:TagRole", "iam:TagPolicy", "iam:CreatePolicy", "iam:DeletePolicy",
                "iam:GetPolicy", "iam:GetPolicyVersion", "iam:ListEntitiesForPolicy",
                "iam:CreateOpenIDConnectProvider", "iam:DeleteOpenIDConnectProvider",
                "iam:GetOpenIDConnectProvider", "iam:TagOpenIDConnectProvider"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": "iam:PassRole",
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "iam:PassedToService": ["eks.amazonaws.com", "ec2.amazonaws.com"]
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": "iam:CreateServiceLinkedRole",
            "Resource": "*"
        }
    ]
}
```

5. Name it `FlashSaleDeployPolicy` → **Create policy**
6. Go back to user creation → search and select `FlashSaleDeployPolicy` → **Create user**
7. Click the user → **Security credentials** → **Create access key** → **CLI**
8. **Save the Access Key ID and Secret Access Key**

---

### Step 2: Configure AWS CLI

```powershell
aws configure
# Enter:
#   Access Key ID: <from Step 1>
#   Secret Access Key: <from Step 1>
#   Region: ap-southeast-1
#   Output: json

# Verify
aws sts get-caller-identity
```

---

### Step 3: Deploy with Terraform

```powershell
cd terraform

# Create config file
Copy-Item terraform.tfvars.example terraform.tfvars

# Edit terraform.tfvars:
# db_password = "YourDBPassword123!"
# jwt_secret  = "YourJWTSecret123!"

# Deploy (takes ~20 minutes)
terraform init
terraform apply
# Type "yes" when prompted
```

---

### Step 4: Configure kubectl

```powershell
# Connect to EKS cluster
aws eks update-kubeconfig --name flashsale-cluster --region ap-southeast-1

# Test connection
kubectl get nodes
```

**If you get "Unauthorized" error:**

```powershell
$USER_ARN = (aws sts get-caller-identity --output json | ConvertFrom-Json).Arn

aws eks create-access-entry --cluster-name flashsale-cluster --principal-arn $USER_ARN --region ap-southeast-1

aws eks associate-access-policy --cluster-name flashsale-cluster --principal-arn $USER_ARN --policy-arn arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy --access-scope type=cluster --region ap-southeast-1
```

---

## Part 2: Deploy Application

### Step 5: Build & Push Docker Images

```powershell
# Make sure Docker Desktop is running!

$REGION = "ap-southeast-1"
$ACCOUNT_ID = (aws sts get-caller-identity --output json | ConvertFrom-Json).Account
$ECR_URL = "$ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com"

# Login to ECR
aws ecr get-login-password --region $REGION | docker login --username AWS --password-stdin $ECR_URL

# Build and push all services
cd flashsale
$services = "api-gateway", "user-service", "product-service", "order-service", "purchase-service", "order-worker"

foreach ($svc in $services) {
    docker build -f "deployments/Dockerfile.$svc" -t "$ECR_URL/flashsale/$svc`:latest" .
    docker push "$ECR_URL/flashsale/$svc`:latest"
    Write-Host "✅ $svc pushed"
}
cd ..
```

---

### Step 6: Update Kubernetes Configs

```powershell
cd k8s

# Get terraform outputs
cd ..\terraform
$RDS = terraform output -raw postgres_connection_host
$REDIS = terraform output -raw redis_endpoint
$SQS = terraform output -raw sqs_order_queue_url
$ROLE = terraform output -raw workload_role_arn
$ALB_ROLE = terraform output -raw aws_load_balancer_controller_role_arn
$VPC = terraform output -raw vpc_id
cd ..\k8s

# Display values (copy these to update configs manually if needed)
Write-Host "RDS: $RDS"
Write-Host "Redis: $REDIS"
Write-Host "SQS: $SQS"
Write-Host "Role: $ROLE"
```

**Edit `k8s/configmap.yaml`:**
- Set `DB_HOST` to your RDS endpoint
- Set `REDIS_ADDR` to your Redis endpoint with `:6379`
- Set `SQS_QUEUE_URL` to your SQS URL

**Edit `k8s/service-account.yaml`:**
- Set `eks.amazonaws.com/role-arn` to your workload role ARN

---

### Step 7: Install ALB Controller

```powershell
# Update ingress.yaml with role ARN
(Get-Content ingress.yaml) -replace 'REPLACE_WITH_ALB_CONTROLLER_ROLE_ARN', $ALB_ROLE | Set-Content ingress.yaml

# Add Helm repo
helm repo add eks https://aws.github.io/eks-charts
helm repo update

# Install controller
helm install aws-load-balancer-controller eks/aws-load-balancer-controller `
    --namespace kube-system `
    --set clusterName=flashsale-cluster `
    --set serviceAccount.create=false `
    --set serviceAccount.name=aws-load-balancer-controller `
    --set region=ap-southeast-1 `
    --set vpcId=$VPC

# Apply ingress service account
kubectl apply -f ingress.yaml
```

---

### Step 8: Deploy to Kubernetes

```powershell
# Create namespace and secrets
kubectl apply -f namespace.yaml

kubectl create secret generic flashsale-secrets `
    --namespace flashsale `
    --from-literal=jwt-secret=YourJWTSecret123! `
    --from-literal=db-password=YourDBPassword123! `
    --from-literal=db-username=flashsale

# Deploy all services
kubectl apply -f service-account.yaml
kubectl apply -f configmap.yaml
kubectl apply -f services/
kubectl apply -f ingress.yaml

# Watch pods start
kubectl get pods -n flashsale -w
```

---

### Step 9: Verify Deployment

```powershell
# Check pods
kubectl get pods -n flashsale

# Get ALB URL (may take 3-5 minutes)
kubectl get ingress -n flashsale

# Get and test URL
$API_URL = kubectl get ingress flashsale-ingress -n flashsale -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
Write-Host "Your API: http://$API_URL"
curl "http://$API_URL/health"
```

**Expected:** `{"status":"ok"}`

---

## Part 3: Cleanup & Deletion

> ⚠️ **AWS charges money every hour!** Always destroy when done testing.

### Quick Destroy (Recommended)

```powershell
cd terraform
terraform destroy
# Type "yes" when prompted
```

This deletes **everything** automatically.

---

### Manual Deletion (Step by Step)

If `terraform destroy` fails, delete resources manually in this order:

#### 1. Delete Kubernetes Resources

```powershell
kubectl delete -f k8s/ingress.yaml
kubectl delete -f k8s/services/
kubectl delete -f k8s/configmap.yaml
kubectl delete -f k8s/service-account.yaml
kubectl delete namespace flashsale

# Uninstall ALB controller
helm uninstall aws-load-balancer-controller -n kube-system
```

#### 2. Delete via Terraform

```powershell
cd terraform
terraform destroy
```

#### 3. If Terraform Destroy Fails - AWS Console Cleanup

Delete resources in this order via [AWS Console](https://console.aws.amazon.com/):

| Order | Service | What to Delete | AWS Console Location |
|-------|---------|----------------|---------------------|
| 1 | **EKS** | flashsale-cluster | EKS → Clusters → Delete |
| 2 | **EC2** | Load Balancers (ALB/NLB) | EC2 → Load Balancers → Delete |
| 3 | **RDS** | flashsale-db | RDS → Databases → Delete (skip snapshot) |
| 4 | **ElastiCache** | flashsale-redis | ElastiCache → Redis → Delete |
| 5 | **SQS** | flashsale-order-events | SQS → Queues → Delete |
| 6 | **ECR** | flashsale/* repositories | ECR → Repositories → Delete |
| 7 | **Secrets Manager** | flashsale/* | Secrets Manager → Delete (immediate) |
| 8 | **IAM** | flashsale-* roles/policies | IAM → Roles/Policies → Delete |
| 9 | **VPC** | flashsale-vpc | VPC → Your VPCs → Delete |

**Important VPC Cleanup:**
- Delete NAT Gateways first (takes 5 min)
- Release Elastic IPs
- Delete Internet Gateway
- Then delete VPC

#### 4. Verify Complete Cleanup

Check these services have no flashsale resources:

```powershell
# Check for remaining resources
aws eks list-clusters --region ap-southeast-1
aws rds describe-db-instances --region ap-southeast-1
aws elasticache describe-replication-groups --region ap-southeast-1
aws sqs list-queues --region ap-southeast-1
```

---

## Troubleshooting

### ❌ kubectl: "Unauthorized"

```powershell
$USER_ARN = (aws sts get-caller-identity --output json | ConvertFrom-Json).Arn
aws eks create-access-entry --cluster-name flashsale-cluster --principal-arn $USER_ARN --region ap-southeast-1
aws eks associate-access-policy --cluster-name flashsale-cluster --principal-arn $USER_ARN --policy-arn arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy --access-scope type=cluster --region ap-southeast-1
```

### ❌ Docker: "Cannot connect to Docker daemon"

Start Docker Desktop and wait for it to fully initialize.

### ❌ ALB not getting address

```powershell
# Check controller logs
kubectl logs -l app.kubernetes.io/name=aws-load-balancer-controller -n kube-system --tail=50

# Restart controller
kubectl rollout restart deployment aws-load-balancer-controller -n kube-system
```

### ❌ Pods in CrashLoopBackOff

```powershell
kubectl logs <pod-name> -n flashsale --previous
kubectl describe pod <pod-name> -n flashsale
```

### ❌ Secrets already exist

```powershell
kubectl delete secret flashsale-secrets -n flashsale
# Then recreate
```

---

## Cost Estimate

| Duration | Estimated Cost |
|----------|---------------|
| 1 day | ~$5 |
| 3 days | ~$15 |
| 1 week | ~$30 |

**Main costs:** EKS control plane ($0.10/hr), RDS ($0.02/hr), ElastiCache ($0.02/hr), NAT Gateway ($0.05/hr)
