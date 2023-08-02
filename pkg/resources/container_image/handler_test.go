package container_image

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/aws/aws-lambda-go/cfn"
)

func mustParse(s string) name.Reference {
	result, err := name.ParseReference(s)
	if err != nil {
		log.Fatalf("%s", err)
	}
	return result
}

func Test_validate(t *testing.T) {
	type args struct {
		event cfn.Event
	}
	tests := []struct {
		name           string
		args           args
		want           *resourceProperties
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "ValidRepositorAr",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/mesosphere/aws-cli:latest",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/mesosphere/aws-cli",
					},
				},
			},
			want: &resourceProperties{
				Source:         mustParse("mesosphere/aws-cli:latest"),
				Target:         mustParse("444093529715.dkr.ecr.eu-central-1.amazonaws.com/mesosphere/aws-cli:latest"),
				Region:         "eu-central-1",
				AccountID:      "444093529715",
				RepositoryName: "mesosphere/aws-cli",
				SourceTag:      "latest",
				SourceDigest:   "",
				SourceName:     "docker.io/mesosphere/aws-cli",
			},
			wantErr:        false,
			wantErrMessage: "",
		},

		{
			name: "DigestWithoutTag",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/library/python@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/python",
					},
				},
			},
			want: &resourceProperties{
				Source:         mustParse("python@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659"),
				Target:         mustParse("444093529715.dkr.ecr.eu-central-1.amazonaws.com/python@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659"),
				Region:         "eu-central-1",
				AccountID:      "444093529715",
				RepositoryName: "python",
				SourceTag:      "",
				SourceDigest:   "sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
				SourceName:     "docker.io/library/python",
			},
			wantErr:        false,
			wantErrMessage: "",
		},

		{
			name: "DigestWithTag",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/library/python:3.9@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/python",
					},
				},
			},
			want: &resourceProperties{
				Source:         mustParse("python@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659"),
				Target:         mustParse("444093529715.dkr.ecr.eu-central-1.amazonaws.com/python:3.9"),
				Region:         "eu-central-1",
				AccountID:      "444093529715",
				RepositoryName: "python",
				SourceTag:      "3.9",
				SourceDigest:   "sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
				SourceName:     "docker.io/library/python",
			},
			wantErr: false,
		},
		{
			name: "TagOnly",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/library/python:3.9",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/python",
					},
				},
			},
			want: &resourceProperties{
				Source:         mustParse("python:3.9"),
				Target:         mustParse("444093529715.dkr.ecr.eu-central-1.amazonaws.com/python:3.9"),
				Region:         "eu-central-1",
				AccountID:      "444093529715",
				RepositoryName: "python",
				SourceTag:      "3.9",
				SourceName:     "docker.io/library/python",
			},
			wantErr: false,
		},
		{
			name: "IncorrectName",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "https://docker.io/library/python:3.9",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/python",
					},
				},
			},
			want:           nil,
			wantErr:        true,
			wantErrMessage: "https://docker.io/library/python:3.9: invalid reference format",
		},
		{
			name: "IncorrectARN",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/library/python:3.9",
						"RepositoryArn":  "arn:a:ws:ecr:eu-ce:ntral-1:444093529715:repository/python",
					},
				},
			},
			want:           nil,
			wantErr:        true,
			wantErrMessage: "Invalid AWS ECR repository ARN: arn:a:ws:ecr:eu-ce:ntral-1:444093529715:repository/python",
		},
		{
			name: "MissingReference",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{},
				},
			},
			want:           nil,
			wantErr:        true,
			wantErrMessage: "ImageReference is missing or not a string",
		},
		{
			name: "MissingARN",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/library/python:3.9",
					},
				},
			},
			want:           nil,
			wantErr:        true,
			wantErrMessage: "RepositoryArn is missing or not a string",
		},
		{
			name: "LatestAndDigest",
			args: args{
				event: cfn.Event{
					ResourceProperties: map[string]interface{}{
						"ImageReference": "public.ecr.aws/docker/library/alpine:latest@sha256:82d1e9d7ed48a7523bdebc18cf6290bdb97b82302a8a9c27d4fe885949ea94d1",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/alpine",
					},
				},
			},
			want: &resourceProperties{
				Source:         mustParse("public.ecr.aws/docker/library/alpine@sha256:82d1e9d7ed48a7523bdebc18cf6290bdb97b82302a8a9c27d4fe885949ea94d1"),
				Target:         mustParse("444093529715.dkr.ecr.eu-central-1.amazonaws.com/alpine:latest"),
				Region:         "eu-central-1",
				AccountID:      "444093529715",
				RepositoryName: "alpine",
				SourceTag:      "latest",
				SourceDigest:   "sha256:82d1e9d7ed48a7523bdebc18cf6290bdb97b82302a8a9c27d4fe885949ea94d1",
				SourceName:     "public.ecr.aws/docker/library/alpine",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validate(tt.args.event)
			if err != nil && tt.wantErrMessage != "" && tt.wantErrMessage != err.Error() {
				t.Errorf("validate() error = %v, wantErrMessage %v", err, tt.wantErrMessage)
				return
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validate() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_handler(t *testing.T) {
	type args struct {
		ctx   context.Context
		event cfn.Event
	}
	tests := []struct {
		name                   string
		args                   args
		wantPhysicalResourceID string
		wantData               map[string]interface{}
		wantErr                bool
		wantErrMessage         string
	}{
		{
			name: "withTagAndDigest",
			args: args{
				ctx: context.Background(),
				event: cfn.Event{
					ResourceType: "Custom::ContainerImage",
					RequestType:  "Create",
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/library/python:3.9@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/cfn-container-image-provider-demo",
					},
				},
			},
			wantPhysicalResourceID: "444093529715.dkr.ecr.eu-central-1.amazonaws.com/cfn-container-image-provider-demo:3.9",
			wantErr:                false,
		},

		{
			name: "DockerHubName",
			args: args{
				ctx: context.Background(),
				event: cfn.Event{
					ResourceType: "Custom::ContainerImage",
					RequestType:  "Create",
					ResourceProperties: map[string]interface{}{
						"ImageReference": "python:3.7",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/cfn-container-image-provider-demo",
					},
				},
			},
			wantPhysicalResourceID: "444093529715.dkr.ecr.eu-central-1.amazonaws.com/cfn-container-image-provider-demo:3.7",
			wantErr:                false,
		},
		{
			name: "DigestOnly",
			args: args{
				ctx: context.Background(),
				event: cfn.Event{
					ResourceType: "Custom::ContainerImage",
					RequestType:  "Create",
					ResourceProperties: map[string]interface{}{
						"ImageReference": "docker.io/library/python@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/cfn-container-image-provider-demo",
					},
				},
			},
			wantPhysicalResourceID: "444093529715.dkr.ecr.eu-central-1.amazonaws.com/cfn-container-image-provider-demo@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
			wantErr:                false,
			wantErrMessage:         "",
		},
		{
			name: "NameOnly",
			args: args{
				ctx: context.Background(),
				event: cfn.Event{
					ResourceType: "Custom::ContainerImage",
					RequestType:  "Create",
					ResourceProperties: map[string]interface{}{
						"ImageReference": "python",
						"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/cfn-container-image-provider-demo",
					},
				},
			},
			wantPhysicalResourceID: "444093529715.dkr.ecr.eu-central-1.amazonaws.com/cfn-container-image-provider-demo:latest",
			wantErr:                false,
			wantErrMessage:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPhysicalResourceID, gotData, err := Handler(tt.args.ctx, tt.args.event)

			if err != nil && tt.wantErrMessage != "" && tt.wantErrMessage != err.Error() {
				t.Errorf("handler() error = %v, wantErrMessage %v", err, tt.wantErrMessage)
				return
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("handler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if gotData == nil {
					t.Errorf("handler() error, no data returned")
					return
				}
				if digest, ok := gotData["Digest"].(string); ok {
					if !regexp.MustCompile("^sha256:[a-f0-9]+$").MatchString(digest) {
						t.Errorf("handler() error digest %s does not match expected regex", digest)
						return
					}
				} else {
					t.Errorf("handler() error, no Digest in wantData")
					return
				}
			}

			if gotPhysicalResourceID != tt.wantPhysicalResourceID {
				t.Errorf("handler() gotPhysicalResourceID = %v, want %v", gotPhysicalResourceID, tt.wantPhysicalResourceID)
			}

			tt.args.event.RequestType = "Delete"
			tt.args.event.PhysicalResourceID = gotPhysicalResourceID
			_, _, err = Handler(tt.args.ctx, tt.args.event)
			if err != nil {
				t.Errorf("handler() error = %v", err)
				return
			}
		})
	}
}

func Test_tagging_image(t *testing.T) {
	var err error
	var awsSession *session.Session
	var ecrService *ecr.ECR
	var basicAuthentication *authn.Basic

	if awsSession, err = session.NewSessionWithOptions(
		session.Options{SharedConfigState: session.SharedConfigEnable}); err != nil {
		t.Fatal(err)
	} else {
		ecrService = ecr.New(awsSession)
	}

	if basicAuthentication, err = getAuthentication(ecrService); err != nil {
		t.Fatal(err)
	}

	pullOptions := []remote.Option{
		remote.WithAuth(basicAuthentication),
		remote.WithContext(context.Background()),
	}

	digests := []string{
		"sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659",
		"sha256:2e94e493d6d5010d739ea473e44ea40f7c6e168bcb78e0c5a48c64f06aafbf5f",
	}

	for i := 0; i < 2; i++ {
		for _, digest := range digests {
			request := cfn.Event{
				ResourceType: "Custom::ContainerImage",
				RequestType:  "Create",
				ResourceProperties: map[string]interface{}{
					"ImageReference": fmt.Sprintf("docker.io/library/python:3.9@%s", digest),
					"RepositoryArn":  "arn:aws:ecr:eu-central-1:444093529715:repository/cfn-container-image-provider-demo",
				},
			}

			physicalResourceId, digestResult, err := Handler(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if d, ok := digestResult["Digest"].(string); !ok || d != digest {
				t.Logf("incorrect digest returned:\ngot: %s\nexp: %s\n", d, digest)
			}
			descriptor, err := remote.Get(mustParse(physicalResourceId), pullOptions...)
			if err != nil {
				t.Fatal(err)
			}
			if descriptor.Digest.String() != digest {
				t.Logf("got: %s\nexp: %s\n", descriptor.Digest, digest)
			}
		}
	}
}
