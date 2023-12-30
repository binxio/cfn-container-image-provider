package container_image

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/logs"

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
	SourceName     string
	Platform       *v1.Platform
	Target         name.Reference
	Region         string
	AccountID      string
	RepositoryName string
}

// The name must start with a letter and can only contain lowercase letters, numbers, hyphens, underscores, periods and forward slashes.
var ecrRepositoryArnPattern = regexp.MustCompile(`^arn:aws:ecr:([a-z\d-]+):(\d+):repository/([a-z][a-z\d-_/.]+)$`)

func validate(event cfn.Event) (*resourceProperties, error) {
	var err error
	var imageReference reference.Reference
	result := new(resourceProperties)

	if ref, ok := event.ResourceProperties["ImageReference"].(string); ok {
		imageReference, err = reference.Parse(ref)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", ref, err)
		}

	} else {
		return nil, fmt.Errorf("ImageReference is missing or not a string")
	}

	result.Source, err = name.ParseReference(reference.FamiliarString(imageReference))
	if err != nil {
		return nil, err
	}

	parts := reference.ReferenceRegexp.FindStringSubmatch(imageReference.String())
	if len(parts) == 0 {
		return nil, fmt.Errorf("reference.ReferenceRegexp failed to match %s", imageReference)
	}
	if len(parts) > 3 {
		result.SourceDigest = parts[3]
	}
	if len(parts) > 2 {
		result.SourceTag = parts[2]
	}
	if len(parts) > 1 {
		result.SourceName = parts[1]
	}

	if result.SourceDigest == "" && result.SourceTag == "" {
		result.SourceTag = "latest"
	}

	if result.SourceDigest != "" {
		var digestReference reference.Reference
		digestReference, err = reference.Parse(fmt.Sprintf("%s@%s", result.SourceName, result.SourceDigest))
		if err != nil {
			return nil, fmt.Errorf("failed to turn source reference into a digest reference %s@%s, %s", result.SourceName, result.SourceDigest, err)
		}
		if result.Source, err = name.ParseReference(reference.FamiliarString(digestReference)); err != nil {
			return nil, fmt.Errorf("failed to turn source reference into a digest reference %s, %s", digestReference, err)
		}
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

	if platform, ok := event.ResourceProperties["Platform"].(string); ok {
		if strings.TrimSpace(strings.ToLower(platform)) == "all" {
			result.Platform = nil
		} else {
			result.Platform, err = v1.ParsePlatform(platform)
			if err != nil {
				return nil, fmt.Errorf("invalid Platform format, %s", err)
			}
		}
	} else {
		// backwards compatible with first release
		result.Platform = &v1.Platform{OS: "linux", Architecture: "amd64"}
	}
	return result, nil
}

func create(ctx context.Context, event cfn.Event, authenticator authn.Authenticator) (physicalResourceID string, data map[string]interface{}, err error) {
	var properties *resourceProperties
	if properties, err = validate(event); err != nil {
		return "", nil, err
	}

	pullOptions := []remote.Option{
		remote.WithContext(ctx),
	}
	if properties.Platform != nil {
		pullOptions = append(pullOptions, remote.WithPlatform(*properties.Platform))
	}

	puller, err := remote.NewPuller(pullOptions...)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create puller for repository: %w", err)
	}

	pushOptions := []remote.Option{
		remote.WithAuth(authenticator),
		remote.WithContext(ctx),
	}

	pusher, err := remote.NewPusher(pushOptions...)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create pusher for repository: %w", err)
	}

	descriptor, err := puller.Get(ctx, properties.Source)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get descriptor for repository: %w", err)
	}

	if properties.Platform == nil {
		err = pusher.Push(ctx, properties.Target, descriptor)
		if err != nil {
			return "", nil, fmt.Errorf("failed to push descriptor: %w", err)
		}
	} else {
		image, err := descriptor.Image()
		if err != nil {
			return "", nil, fmt.Errorf("failed to get the platform specific image from descriptor: %w", err)
		}
		err = pusher.Push(ctx, properties.Target, image)
		if err != nil {
			return "", nil, fmt.Errorf("failed to push image: %w", err)
		}
	}

	var platforms []string
	if properties.Platform != nil {
		platforms = []string{properties.Platform.String()}
	} else {
		platforms = getPlatforms(descriptor)
	}

	data = map[string]interface{}{
		"Digest":         descriptor.Digest.String(),
		"ImageReference": properties.Target.String(),
		"Platforms":      platforms,
	}

	return properties.Target.String(), data, nil
}

func getPlatforms(descriptor *remote.Descriptor) (platforms []string) {
	platforms = make([]string, 0)

	if index, err := descriptor.ImageIndex(); err == nil {
		if indexManifest, err := index.IndexManifest(); err == nil {
			for _, manifest := range indexManifest.Manifests {
				if manifest.Platform != nil {
					platforms = append(platforms, manifest.Platform.String())
				}
			}
		}
	}
	return
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

	logs.Warn.SetOutput(os.Stderr)
	logs.Progress.SetOutput(os.Stderr)

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
