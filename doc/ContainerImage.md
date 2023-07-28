# Custom::ContainerImage
The `ContainerImage` resource clones a copy of a public container image into an ECR repository.


## Syntax
To clone a ContainerImage in your AWS CloudFormation template, use the following syntax:

```yaml
  Python39:
    Type: 'Custom::ContainerImage'
    Properties:
      ImageReference: python:3.9@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659
      RepositoryArn: !GetAtt Repository.Arn
      ServiceToken: !Sub 'arn:aws:lambda:${AWS::Region}:${AWS::AccountId}:function:cfn-container-image-provider'
```
This will clone the image python with digest sha256:3d... to the repository and tag it with 3.9.
The previously tagged image will remain available.

## Properties
You must specify the following properties:

| Name            | Description                                        |
|-----------------|----------------------------------------------------|
| ImageReference  | container image reference with tag, digest or both |
| RepositoryArn   | ARN of the ECR repository to clone the image to    |

To force an update, use add the digest of the image you want.

## Return values
The ContainerImage returns the container reference of the image in the ECR repository.



With 'Fn::GetAtt' the following values are available:

| Name            | Description             |
|-----------------|-------------------------|
| Digest          | the digest of the image |
