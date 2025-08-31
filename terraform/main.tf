terraform {
required_providers {
aws = {
source  = "hashicorp/aws"
version = "~> 5.0"
}
}
}
provider "aws" {
region = "eu-west-3"
}
module "vpc" {
source  = "terraform-aws-modules/vpc/aws"
version = "5.8.0"
name    = "chatpay-vpc"
cidr    = "10.0.0.0/16"
azs     = ["eu-west-3a", "eu-west-3b", "eu-west-3c"]
private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
enable_nat_gateway = true
single_nat_gateway = true
}
module "eks" {
source  = "terraform-aws-modules/eks/aws"
version = "19.15.3"
cluster_name = "chatpay-eks"
cluster_version = "1.29"
vpc_id = module.vpc.vpc_id
subnet_ids = module.vpc.private_subnets
cluster_endpoint_public_access = true
eks_managed_node_groups = {
default = {
instance_types = ["t2.micro"]
min_size     = 1
max_size     = 3
desired_size = 2
}
}
}
resource "aws_db_instance" "postgres_new" {
identifier = "chatpay-postgres-new"
engine = "postgres"
engine_version = "16"
instance_class = "db.t3.micro"
allocated_storage = 20
storage_type = "gp2"
username = "chatpay"
password = "FirstPboss00."
vpc_security_group_ids = [aws_security_group.postgres.id]
db_subnet_group_name = aws_db_subnet_group.postgres_public.name
multi_az = false
publicly_accessible = true
apply_immediately = true
}
resource "aws_db_subnet_group" "postgres_public" {
name = "chatpay-postgres-public"
subnet_ids = module.vpc.public_subnets
}
resource "aws_security_group" "postgres" {
vpc_id = module.vpc.vpc_id
ingress {
from_port = 5432
to_port = 5432
protocol = "tcp"
cidr_blocks = ["0.0.0.0/0"] # Allow from anywhere (for testing, replace with your IP for production)
}
}
resource "aws_msk_cluster" "kafka" {
cluster_name = "chatpay-kafka"
kafka_version = "3.6.0"
number_of_broker_nodes = 3
broker_node_group_info {
instance_type = "kafka.t3.small"
client_subnets = module.vpc.private_subnets
security_groups = [aws_security_group.kafka.id]
storage_info {
ebs_storage_info {
volume_size = 100
}
}
}
}
resource "aws_security_group" "kafka" {
vpc_id = module.vpc.vpc_id
ingress {
from_port = 9092
to_port = 9092
protocol = "tcp"
cidr_blocks = ["10.0.0.0/16"]
}
}
resource "aws_elasticache_cluster" "redis" {
cluster_id = "chatpay-redis"
engine = "redis"
node_type = "cache.t3.micro"
num_cache_nodes = 1
subnet_group_name = aws_elasticache_subnet_group.redis.name
security_group_ids = [aws_security_group.redis.id]
}
resource "aws_elasticache_subnet_group" "redis" {
name = "chatpay-redis"
subnet_ids = module.vpc.private_subnets
}
resource "aws_security_group" "redis" {
vpc_id = module.vpc.vpc_id
ingress {
from_port = 6379
to_port = 6379
protocol = "tcp"
cidr_blocks = ["10.0.0.0/16"]
}
}
