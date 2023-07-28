package container_image

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	reference "github.com/docker/distribution/reference"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type resourceProperties struct {
	Source         name.Reference
	SourceTag      string
	SourceDigest   string
	Target         name.Reference
	Region         string
	AccountID      string
	RepositoryName string
}

var ecrRepositoryArnPattern = regexp.MustCompile(`^arn:aws:ecr:([a-z\d-]+):(\d+):repository/([a-zA-Z\d-_]+)$`)

func validate(event cfn.Event) (*resourceProperties, error) {
	var err error
	result := new(resourceProperties)

	var imageReference reference.Reference
	if ref, ok := event.ResourceProperties["ImageReference"].(string); ok {
		imageReference, err = reference.Parse(ref)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", ref, err)
		}

	} else {
		return nil, fmt.Errorf("ImageReference is missing or not a string")
	}

	if tagged, ok := imageReference.(reference.Tagged); ok {
		result.SourceTag = tagged.Tag()
	} else {
		if digestRef, ok := imageReference.(reference.Digested); ok {
			result.SourceDigest = digestRef.Digest().String()
		} else {
			result.SourceTag = "latest"
		}
	}

	result.Source, err = name.ParseReference(reference.FamiliarString(imageReference))
	if err != nil {
		return nil, err
	}

	if arn, ok := event.ResourceProperties["RepositoryArn"].(string); ok {
		matches := ecrRepositoryArnPattern.FindStringSubmatch(arn)
		if len(matches) != 4 {
			return nil, fmt.Errorf("Invalid AWS ECR repository ARN: %s", arn)
		}

		result.Region = matches[1]
		result.AccountID = matches[2]
		result.RepositoryName = matches[3]

		var reference string
		if result.SourceTag != "" {
			reference = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
				result.AccountID,
				result.Region,
				result.RepositoryName,
				result.SourceTag)
		} else {
			reference = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s@%s",
				result.AccountID,
				result.Region,
				result.RepositoryName,
				result.SourceDigest)
		}

		if result.Target, err = name.ParseReference(reference); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("RepositoryArn is missing or not a string")
	}
	return result, nil
}

func reportProgress(action string) func(chan v1.Update) {
	return func(progress chan v1.Update) {
		var previousPercentage int64
		for p := range progress {
			var percentage int64
			if p.Total != 0 {
				percentage = p.Complete * 100 / p.Total
			}
			if previousPercentage != percentage {
				log.Printf("%s: %d%%\n", action, percentage)
				previousPercentage = percentage
			}
			if p.Error != nil {
				log.Printf("%s failed: %s", action, p.Error)
			}
		}
	}
}

func create(ctx context.Context, event cfn.Event, authenticator authn.Authenticator) (physicalResourceID string, data map[string]interface{}, err error) {
	var properties *resourceProperties
	if properties, err = validate(event); err != nil {
		return "", nil, err
	}

	pullProgress := make(chan v1.Update)
	go reportProgress("Pull")(pullProgress)

	pullOptions := []remote.Option{
		remote.WithProgress(pullProgress),
		remote.WithContext(ctx),
	}
	image, err := remote.Image(properties.Source, pullOptions...)
	if err != nil {
		return "", nil, fmt.Errorf("failed to pull the Docker image: %w", err)
	}

	pushProgress := make(chan v1.Update)
	go reportProgress("Push")(pushProgress)

	pushOptions := []remote.Option{
		remote.WithProgress(pushProgress),
		remote.WithAuth(authenticator),
		remote.WithContext(ctx),
	}
	if err = remote.Write(properties.Target, image, pushOptions...); err != nil {
		return "", nil, fmt.Errorf("failed to push the Docker image to ECR: %w", err)
	}

	if digest, err := image.Digest(); err == nil {
		data = map[string]interface{}{
			"Digest": digest.String(),
		}
	} else {
		return properties.Target.String(), nil, fmt.Errorf("failed to obtain digest of image: %s", err)
	}

	return properties.Target.String(), data, nil
}

func delete(ctx context.Context, event cfn.Event, authenticator authn.Authenticator) (physicalResourceID string, data map[string]interface{}, err error) {
	var imageReference name.Reference
	if imageReference, err = name.ParseReference(event.PhysicalResourceID); err == nil {
		deleteOptions := []remote.Option{
			remote.WithAuth(authenticator),
			remote.WithContext(ctx),
		}
		if err = remote.Delete(imageReference, deleteOptions...); err != nil {
			log.Printf("ignoring failed delete of image %s, %s", event.PhysicalResourceID, err)
		}
	} else {
		log.Printf("ignoring invalid physical resource id %s", event.PhysicalResourceID)
	}
	return physicalResourceID, nil, nil
}

func getAuthentication(svc *ecr.ECR) (*authn.Basic, error) {
	var err error
	var response *ecr.GetAuthorizationTokenOutput
	response, err = svc.GetAuthorizationToken(&ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return nil, err
	}
	if len(response.AuthorizationData) == 0 || response.AuthorizationData[0].AuthorizationToken == nil {
		return nil, fmt.Errorf("no authorization data was returned")
	}

	var token []byte
	token, err = base64.StdEncoding.DecodeString(*response.AuthorizationData[0].AuthorizationToken)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(string(token), ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("token seperated by : contains %d elements, not 2", len(parts))
	}

	return &authn.Basic{Username: parts[0], Password: parts[1]}, nil
}

func Handler(ctx context.Context, event cfn.Event) (physicalResourceID string, data map[string]interface{}, err error) {
	var awsSession *session.Session
	var ecrService *ecr.ECR
	var basicAuthentication *authn.Basic

	if awsSession, err = session.NewSessionWithOptions(
		session.Options{SharedConfigState: session.SharedConfigEnable}); err != nil {
		return "", nil, err
	} else {
		ecrService = ecr.New(awsSession)
	}

	if basicAuthentication, err = getAuthentication(ecrService); err != nil {
		return "", nil, err
	}

	if strings.Compare(event.ResourceType, "Custom::ContainerImage") == 0 {
		switch event.RequestType {
		case cfn.RequestCreate:
			physicalResourceID, data, err = create(ctx, event, basicAuthentication)
			if physicalResourceID == "" {
				physicalResourceID = "create-failed"
			}
			return physicalResourceID, data, err
		case cfn.RequestUpdate:
			return create(ctx, event, basicAuthentication)
		case cfn.RequestDelete:
			return delete(ctx, event, basicAuthentication)
		default:
			return "", nil, fmt.Errorf("unsupported request type: %s", event.RequestType)
		}
	}
	return "", nil, fmt.Errorf("unsupported resource type: %s", event.ResourceType)
}
