package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/heptiolabs/healthcheck"
	clientset "github.com/objectrocket/tiny-operator/pkg/client/clientset/versioned"
	"k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func init() {
	flag.StringVar(&kubeCfgFile, "kubecfg-file", "", "location of kubecfg file")
	flag.StringVar(&image, "image", "jessewiles/echo-chamber:0.1", "docker image to run")
	flag.Parse()
}

func main() {
	health := healthcheck.NewMetricsHandler(r, "tiny-operator")
	mux := http.NewServeMux()
	mux.HandleFunc("/live", health.LiveEndpoint)
	mux.HandleFunc("/ready", health.ReadyEndpoint)

	srv := &http.Server{Handler: mux}

	doneChan := make(chan struct{})
	var wg sync.WaitGroup

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-signalChan:
			log.Error("Received shutdown signal...")
			close(doneChan)
			wg.W()
			os.Exit(0)
		}
	}
}

type Agent struct {
	config    *rest.Config
	crdClient clientset.Interface
	kclient   kubernetes.Interface
	kubeExt   apiextensionsclient.Interface
}

func newAgent(kubeCfgFile) (Agent, error) {
	if kubeCfgFile != "" {
		log.Infof("Using OutOfCluster k8s config with kubeConfigFile: %s", kubeCfgFile)
		config, err := clientcmd.BuildConfigFromFlags("", kubeCfgFile)
		if err != nil {
			panic(err.Error())
		}
	} else {
		config := rest.InClusterConfig()
	}

	// TODO (jwiles): client for generated clientset

	kclient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	kubeExt, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return &Agent{
		config:    config,
		crdClient: nil,
		kclient:   kclient,
		kubeExt:   kubeExt,
	}
}

func initOperator(doneChan chan struct{}, wg *sync.WaitGroup) error {
	err := createCRD()
	if err != nil {
		return err
	}

	wg.Add(1)
	watchEchoEvents(doneChan, wg)

	return nil
}

func monitorEchoEvents(shopchan chan struct{}) (<-chan *echo.EchoServer, <-chan error) {
	events := make(chan *echo.EchoServer)
	errc := make(chan error, 1)

	source := cache.NewListWatchFromClient(crdClient.EchoV1Alpha1().RESTClient(), echo.ResourcePlural, v1.NamespaceAll, fields.Everything())

	createHandler := func(obj interface{}) {
		event := obj.(*echo.EchoServer)
		event.Type = "ADDED"
		events <- event
	}

	deleteHandler := func(obj interface{}) {
		event := obj.(*echo.EchoServer)
		event.Type = "DELETED"
		events <- event
	}

	updateHandler := func(old interface{}, obj interface{}) {
		event := obj.(*echo.EchoServer)
		event.Type = "MODIFIED"
		events <- event
	}

	_, controller := cache.NewInformer(
		source,
		&echo.EchoServer{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    createHandler,
			UpdateFunc: updateHandler,
			DeleteFunc: deleteHandler,
		})

	go controller.Run(stopchan)

	return events, errc
}

func watchEchoEvents(done chan struct{}, wg *sync.WaitGroup) {
	events, watchErrs := monitorEchoEvents(done)
	go func() {
		for {
			select {
			case event := <-events:
				err := handleEchoEvent(event)
				if err != nil {
					log.Error(err)
				}
			case err := <-watchErrs:
				log.Error(err)
			case <-done:
				wg.Done()
				return
			}
		}
	}()
}

func handleEchoEvent(e *echo.EchoServer) error {
	eLock.Lock()
	defer eLock.Unlock()
	switch {
	case e.Type == "ADDED" || e.Type == "MODIFIED":
		return onApplyEcho(e)
	case e.Type == "DELETED":
		return onDeleteEcho(e)
	}
}

func onApplyEcho(e *echo.EchoServer) error {
	log.Infof("Received create event with object: %#v", e)
	return nil
}

func onDeleteEcho(e *echo.EchoServer) error {
	log.Infof("Received delete event with object: %#v", e)
	return nil
}
