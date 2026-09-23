package apf

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// NewRawGetter returns a GET against the API server root that preserves query strings.
func NewRawGetter(cfg *rest.Config) (RawFunc, error) {
	cfg = rest.CopyConfig(cfg)
	cfg.GroupVersion = &schema.GroupVersion{Version: "v1"}
	cfg.APIPath = "/"
	if cfg.NegotiatedSerializer == nil {
		cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}
	client, err := rest.RESTClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("create api server client: %w", err)
	}
	return func(ctx context.Context, path string) ([]byte, error) {
		return client.Get().RequestURI(path).SetHeader("Accept", "*/*").DoRaw(ctx)
	}, nil
}
