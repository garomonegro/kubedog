package main

import (
	"log"

	"github.com/keikoproj/kubedog/pkg/generic"
)

func main() {
	TemplateArguments := []generic.TemplateArgument{
		{
			Key:                 "Namespace",
			EnvironmentVariable: "KUBEDOG_EXAMPLE_NAMESPACE",
			Mandatory:           false,
			Default:             "kubedog-example",
		},
		{
			Key:                 "Image",
			EnvironmentVariable: "KUBEDOG_EXAMPLE_IMAGE",
			Mandatory:           false,
			Default:             "busybox:1.28",
		},
		{
			Key:                 "Message",
			EnvironmentVariable: "KUBEDOG_EXAMPLE_MESSAGE",
			Mandatory:           false,
			Default:             "Hello, Kubedog!",
		},
	}

	args, err := generic.TemplateArgumentsToMap(TemplateArguments...)
	if err != nil {
		log.Fatalln(err)
	}

	templatedInputConfigPath := "templates/pod.yaml"
	_, err = generic.GenerateFileFromTemplate(templatedInputConfigPath, args)
	if err != nil {
		log.Fatalln(err)
	}
}
