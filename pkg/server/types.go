package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/server"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/steve/pkg/accesscontrol"
	steveauth "github.com/rancher/steve/pkg/auth"
	steveserver "github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rancher/harvester/pkg/api"
	"github.com/rancher/harvester/pkg/api/auth"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/controller/crds"
	"github.com/rancher/harvester/pkg/controller/global"
	"github.com/rancher/harvester/pkg/controller/master"
	"github.com/rancher/harvester/pkg/server/ui"
)

type HarvesterServer struct {
	Context context.Context

	RESTConfig    *restclient.Config
	DynamicClient dynamic.Interface
	ClientSet     *kubernetes.Clientset
	ASL           accesscontrol.AccessSetLookup

	steve          *steveserver.Server
	controllers    *steveserver.Controllers
	startHooks     []StartHook
	postStartHooks []func() error

	Handler http.Handler
}

func New(ctx context.Context, clientConfig clientcmd.ClientConfig) (*HarvesterServer, error) {
	var err error
	server := &HarvesterServer{
		Context: ctx,
	}
	server.RESTConfig, err = clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	server.RESTConfig.RateLimiter = ratelimit.None

	if err := Wait(ctx, server.RESTConfig); err != nil {
		return nil, err
	}

	server.ClientSet, err = kubernetes.NewForConfig(server.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes clientset create error: %s", err.Error())
	}

	server.DynamicClient, err = dynamic.NewForConfig(server.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes dynamic client create error:%s", err.Error())
	}

	if err := server.generateSteveServer(); err != nil {
		return nil, err
	}

	ui.ConfigureAPIUI(server.steve.APIServer)

	return server, nil
}

func Wait(ctx context.Context, config *rest.Config) error {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	for {
		_, err := client.Discovery().ServerVersion()
		if err == nil {
			break
		}
		logrus.Infof("Waiting for server to become available: %v", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("startup canceled")
		case <-time.After(2 * time.Second):
		}
	}

	return nil
}

func (s *HarvesterServer) ListenAndServe(listenOpts *dynamiclistener.Config) error {
	opts := &server.ListenOpts{
		Secrets: s.controllers.Core.Secret(),
		TLSListenerConfig: dynamiclistener.Config{
			CloseConnOnCertChange: true,
		},
	}

	if listenOpts != nil {
		opts.TLSListenerConfig = *listenOpts
	}

	return s.steve.ListenAndServe(s.Context, config.HTTPSListenPort, config.HTTPListenPort, opts)
}

// Scaled returns the *config.Scaled,
// it should call after Start.
func (s *HarvesterServer) Scaled() *config.Scaled {
	return config.ScaledWithContext(s.Context)
}

func (s *HarvesterServer) generateSteveServer() error {
	factory, err := controller.NewSharedControllerFactoryFromConfig(s.RESTConfig, Scheme)
	if err != nil {
		return err
	}

	opts := &generic.FactoryOptions{
		SharedControllerFactory: factory,
	}

	var scaled *config.Scaled

	s.Context, scaled, err = config.SetupScaled(s.Context, s.RESTConfig, opts)
	if err != nil {
		return err
	}

	s.controllers, err = steveserver.NewController(s.RESTConfig, opts)
	if err != nil {
		return err
	}

	s.ASL = accesscontrol.NewAccessStore(s.Context, true, s.controllers.RBAC)

	router, err := NewRouter(scaled, s.RESTConfig)
	if err != nil {
		return err
	}

	var authMiddleware steveauth.Middleware
	if !config.SkipAuthentication {
		md := auth.NewMiddleware(scaled)
		authMiddleware = md.ToAuthMiddleware()
	}

	s.steve, err = steveserver.New(s.Context, s.RESTConfig, &steveserver.Options{
		Controllers:     s.controllers,
		AuthMiddleware:  authMiddleware,
		Router:          router.Routes,
		AccessSetLookup: s.ASL,
	})
	if err != nil {
		return err
	}

	s.startHooks = []StartHook{
		crds.Setup,
		master.Setup,
		global.Setup,
		api.Setup,
	}

	s.postStartHooks = []func() error{
		scaled.Start,
	}

	return s.start()
}

func (s *HarvesterServer) start() error {
	for _, hook := range s.startHooks {
		if err := hook(s.Context, s.steve, s.controllers); err != nil {
			return err
		}
	}

	if err := s.controllers.Start(s.Context); err != nil {
		return err
	}

	for _, hook := range s.postStartHooks {
		if err := hook(); err != nil {
			return err
		}
	}

	return nil
}

type StartHook func(context.Context, *steveserver.Server, *steveserver.Controllers) error
