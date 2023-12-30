cfn-container-image-provider - clones container images into a ECR repository
===============================================================================
Allows you to clone public images into your ECR repository, in the following fashion:

```yaml
Resources:
  Repository:
    Type: AWS::ECR::Repository
    Properties:
      RepositoryName: python-clone

  Python37:
    Type: 'Custom::ContainerImage'
    Properties:
      ImageReference: python:3.9
      Platform: all
      RepositoryArn: !GetAtt Repository.Arn
      ServiceToken: !Sub 'arn:aws:lambda:${AWS::Region}:${AWS::AccountId}:function:cfn-container-image-provider'
```
This will copy the multi-architecture repository from python:3.9 to the python-clone repository. If you
want a specific version, add the digest:

```yaml
  Python39:
    Type: 'Custom::ContainerImage'
    Properties:
      Platform: all
      ImageReference: python:3.9@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659
      RepositoryArn: !GetAtt Repository.Arn
      ServiceToken: !Sub 'arn:aws:lambda:${AWS::Region}:${AWS::AccountId}:function:cfn-container-image-provider'
```

If you want a specific platform only, specify Platform too:
```yaml
  Python39:
    Type: 'Custom::ContainerImage'
    Properties:
      ImageReference: python:3.9@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659
      Platform: linux/arm64
      RepositoryArn: !GetAtt Repository.Arn
      ServiceToken: !Sub 'arn:aws:lambda:${AWS::Region}:${AWS::AccountId}:function:cfn-container-image-provider'
```

If you do not specify a platform, linux/amd64 will be used as the default.

## on Resource Delete
When the resource is deleted, the image will be removed too.

## Return Values
The following attributes are returned:

| name           | description                                        |
|----------------|----------------------------------------------------|
| Digest         | the digest hash of the image                       |
| ImageReference | the container image reference name to use in pull  |
| Platforms      | array of platform names availabe in the repository |

When you reference the CFN resource, it will return the ImageReference.

## Installation
To install this custom resource provider, type:

```bash
read -p 'VPC id:' VPC_ID
read -p 'private subnet ids (comma separated):' PRIVATE_SUBNET_IDS
read -p 'security group ids (comma separated):' SECURITY_GROUP_IDS
aws cloudformation create-stack \
       --capabilities CAPABILITY_IAM \
       --stack-name cfn-container-image-provider \
       --template-url s3://binxio-public-eu-central-1/lambdas/cfn-container-image-provider-0.4.0.yaml \
       --parameter-overrides \
          Name=AppVPC,Values=$VPC_ID \
          Name=Subnets,Values=$PRIVATE_SUBNET_IDS \
          Name=SecurityGroupIds,Values=$SECURITY_GROUP_IDS

aws cloudformation wait stack-create-complete \
       --stack-name cfn-container-image-provider
```
or use [![](https://s3.amazonaws.com/cloudformation-examples/cloudformation-launch-stack.png)](https://console.aws.amazon.com/cloudformation/home?region=eu-central-1#/stacks/new?stackName=cfn-container-image-provider&templateURL=https://binxio-public-eu-central-1.s3.amazonaws.com/lambdas/cfn-container-image-provider-0.4.0.yaml)

## Demo
To install a simple sample of the custom ContainerImage resource, type:

```sh
git checkout http://github.com/binxio/cfn-container-image-provider.git
cd cfn-container-image-provider
aws cloudformation deploy \
    --capabilities CAPABILITY_NAMED_IAM \
    --stack-name cfn-container-image-provider-demo \
    --template-body file://cloudformation/demo.yaml
```

