package resource

import (
	"k2p-updater/cmd/app"

	"k8s.io/client-go/dynamic"
)

// Factory creates and initializes resource handlers
type Factory struct {
	Config        *app.Config
	ClientSet     KubeClientInterface
	DynamicClient dynamic.Interface
}

// NewFactory creates a new resource factory
func NewFactory(config *app.Config, clients *app.KubeClients) *Factory {
	return &Factory{
		Config:        config,
		ClientSet:     clients.ClientSet,
		DynamicClient: clients.DynamicClient,
	}
}

// CreateEventHandler creates a new EventInfo instance
func (f *Factory) CreateEventHandler() (Event, error) {
	template, err := ConvertToTemplate(f.Config)
	if err != nil {
		return nil, err
	}

	return &EventInfo{
		Template:   template,
		KubeClient: f.ClientSet,
	}, nil
}

// CreateStatusHandler creates a new StatusInfo instance
func (f *Factory) CreateStatusHandler() (Status, error) {
	template, err := ConvertToTemplate(f.Config)
	if err != nil {
		return nil, err
	}

	return &StatusInfo{
		Template:      template,
		DynamicClient: f.DynamicClient,
	}, nil
}
