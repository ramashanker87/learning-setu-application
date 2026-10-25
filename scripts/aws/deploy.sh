#!/usr/bin/env bash
set -euo pipefail
# Explicit invocation deploys to AWS. No credentials are stored in this repository.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENVIRONMENT="${1:?Usage: deploy.sh staging|production path/to/parameters.json}"
PARAMETERS="${2:?A reviewed CloudFormation parameters JSON file is required}"
[[ "$ENVIRONMENT" == staging || "$ENVIRONMENT" == production ]] || { echo 'Invalid environment' >&2; exit 2; }
PROFILE="${AWS_PROFILE:-jso}"
REGION="${AWS_REGION:-ap-south-1}"
PREFIX="${RESOURCE_PREFIX:-setu-$ENVIRONMENT}"
STACK="${STACK_NAME:-learning-setu-$ENVIRONMENT}"
TAG="${IMAGE_TAG:-$(date -u +%Y%m%dT%H%M%SZ)}"
ACCOUNT="$(aws --profile "$PROFILE" --region "$REGION" sts get-caller-identity --query Account --output text)"
REPOSITORY="$PREFIX-app"
REGISTRY="$ACCOUNT.dkr.ecr.$REGION.amazonaws.com"
IMAGE="$REGISTRY/$REPOSITORY:$TAG"
# Validate required inputs before creating resources or pushing images.
python3 "$ROOT/scripts/aws/parameters.py" "$PARAMETERS" "$ENVIRONMENT" "$PREFIX" "$IMAGE" > /dev/null
if ! aws --profile "$PROFILE" --region "$REGION" ecr describe-repositories --repository-names "$REPOSITORY" >/dev/null 2>&1; then
  aws --profile "$PROFILE" --region "$REGION" ecr create-repository --repository-name "$REPOSITORY" --image-tag-mutability IMMUTABLE --image-scanning-configuration scanOnPush=true >/dev/null
fi
aws --profile "$PROFILE" --region "$REGION" ecr get-login-password | docker login --username AWS --password-stdin "$REGISTRY"
docker build --platform linux/amd64 --target runtime -f "$ROOT/app/Dockerfile" -t "$IMAGE" "$ROOT"
docker push "$IMAGE"
mapfile -t OVERRIDES < <(python3 "$ROOT/scripts/aws/parameters.py" "$PARAMETERS" "$ENVIRONMENT" "$PREFIX" "$IMAGE")
aws --profile "$PROFILE" --region "$REGION" cloudformation deploy \
  --stack-name "$STACK" --template-file "$ROOT/infrastructure/template.yml" \
  --capabilities CAPABILITY_IAM --parameter-overrides "${OVERRIDES[@]}" \
  --no-fail-on-empty-changeset
aws --profile "$PROFILE" --region "$REGION" cloudformation describe-stacks --stack-name "$STACK" --query 'Stacks[0].Outputs' --output table
