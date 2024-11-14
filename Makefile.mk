AWS_REGION=eu-central-1
AWS_ACCOUNT=$(shell aws sts get-caller-identity --query Account --output text)
REGISTRY_HOST=$(AWS_ACCOUNT).dkr.ecr.$(AWS_REGION).amazonaws.com
IMAGE=$(REGISTRY_HOST)/$(NAME)
DOCKER_BUILD_ARGS=
VERSION := $(shell git describe  --tags --dirty)

build:  ## build container image snapshot
	docker build $(DOCKER_BUILD_ARGS) -t $(IMAGE):$(VERSION) . -f Dockerfile

snapshot: build ## build and push container image
	docker push $(IMAGE):$(VERSION)

fmt:    ## formats the source code
	go fmt ./...

test:   ## run unit tests
	go test ./...

test-templates:     ## validate CloudFormation templates
	for n in ./cloudformation/*.yaml ; do aws cloudformation validate-template --template-body file://$$n ; done

deploy-provider: 	 ## deploys the custom provider
	sed -i -e 's^$(NAME):[0-9]*\.[0-9]*\..*^$(NAME):$(VERSION)^g' cloudformation/$(NAME).yaml
	aws cloudformation deploy \
                --capabilities CAPABILITY_IAM \
                --stack-name $(NAME) \
                --template-file ./cloudformation/$(NAME).yaml \
				--no-fail-on-empty-changeset

delete-provider: ## deletes the custom provider
	aws cloudformation delete-stack --stack-name $(NAME)
	aws cloudformation wait stack-delete-complete  --stack-name $(NAME)

deploy-repository:  ## deploys the ECR image repository
	aws cloudformation deploy \
                --capabilities CAPABILITY_IAM \
                --stack-name $(NAME)-ecr-repository \
                --template-file ./cloudformation/ecr-repository.yaml \
				--no-fail-on-empty-changeset

delete-repository:  ## deletes the ECR image repository
	aws cloudformation delete-stack --stack-name $(NAME)-ecr-repository
	aws cloudformation wait stack-delete-complete  --stack-name $(NAME)-ecr-repository


deploy-pipeline:  ## deploys the CI/CD deployment pipeline
	aws cloudformation deploy \
                --capabilities CAPABILITY_IAM \
                --stack-name $(NAME)-pipeline \
                --template-file ./cloudformation/cicd-pipeline.yaml \
                --parameter-overrides S3BucketPrefix=$(S3_BUCKET_PREFIX) \
				--no-fail-on-empty-changeset

delete-pipeline:  ## deletes the CI/CD deployment pipeline
	aws cloudformation delete-stack --stack-name $(NAME)-pipeline
	aws cloudformation wait stack-delete-complete  --stack-name $(NAME)-pipeline

deploy-demo:	## deploys the demo stack
	aws cloudformation deploy --stack-name $(NAME)-demo \
		--template-file ./cloudformation/demo.yaml \
		--capabilities CAPABILITY_NAMED_IAM \
		--no-fail-on-empty-changeset

delete-demo: ## deletes the demo stack
	aws cloudformation delete-stack --stack-name $(NAME)-demo
	aws cloudformation wait stack-delete-complete  --stack-name $(NAME)-demo

tag-patch-release: ## create a tag for a new patch release
	pipenv run git-release-tag bump --level patch

tag-minor-release: ## create a tag for a new minor release
	pipenv run git-release-tag bump --level minor

tag-major-release: ## create a tag for new major release
	pipenv run git-release-tag bump --level major

show-version: ## shows the current version of the workspace
	pipenv run git-release-tag show .

ecr-login:    ## login to the ECR repository
	aws ecr get-login-password --region $(AWS_REGION) | \
	docker login --username AWS --password-stdin $(REGISTRY_HOST)

help:         ## Show this help.
		@egrep -h ':[^#]*##' $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/:[^#]*##/: ##/' -e 's/[ 	]*##[ 	]*/ /' | \
		awk -F: '{printf "%-20s -", $$1; $$1=""; print $$0}'
