package main

import (
	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/binxio/cfn-container-image-provider/pkg/resources/container_image"
)

func main() {
	lambda.Start(cfn.LambdaWrap(container_image.Handler))
}
