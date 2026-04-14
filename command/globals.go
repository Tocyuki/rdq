package command

import "github.com/aws/aws-sdk-go-v2/aws"

type Globals struct {
	Profile         string
	Debug           bool
	AWSConfig       aws.Config
	ClusterArn      string
	SecretArn       string
	Database        string
	BedrockModel    string
	BedrockLanguage string
}
