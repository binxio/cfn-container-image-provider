package container_image

import (
	"context"
	"log"
	"reflect"
	"regexp"
	"testing"

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
				Source:         mustParse("python:3.9@sha256:3d35a404db586d00a4ee5a65fd1496fe019ed4bdc068d436a67ce5b64b8b9659"),
				Target:         mustParse("444093529715.dkr.ecr.eu-central-1.amazonaws.com/python:3.9"),
				Region:         "eu-central-1",
				AccountID:      "444093529715",
				RepositoryName: "python",
				SourceTag:      "3.9",
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
