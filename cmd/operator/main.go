package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/heptiolabs/healthcheck"
	tinyop "github.com/objectrocket/tiny-operator/pkg/apis/tinyop"
	tinyopv1 "github.com/objectrocket/tiny-operator/pkg/apis/tinyop/v1alpha1"
	clientset "github.com/objectrocket/tiny-operator/pkg/client/clientset/versioned"
	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	appVersion  = "0.0.1"
	kubeCfgFile string
	image       string
	eLock       = &sync.Mutex{}
)

func init() {
	flag.StringVar(&kubeCfgFile, "kubecfg-file", "", "location of kubecfg file")
	flag.StringVar(&image, "image", "jessewiles/echo-chamber:0.1", "docker image to run")
	flag.Parse()
}

func main() {
	// Setup and start the operator server process
	// Prometheus and health checks
	r := prometheus.NewRegistry()
	r.MustRegister(prometheus.NewProcessCollector(os.Getpid(), ""))
	r.MustRegister(prometheus.NewGoCollector())

	health := healthcheck.NewMetricsHandler(r, "tiny-operator")
	mux := http.NewServeMux()
	mux.HandleFunc("/live", health.LiveEndpoint)
	mux.HandleFunc("/ready", health.ReadyEndpoint)

	srv := &http.Server{Handler: mux}
	l, err := net.Listen("tcp", ":8000")
	if err != nil {
		log.Fatal(err)
	}
	go srv.Serve(l)

	doneChan := make(chan struct{})
	var wg sync.WaitGroup

	err = initOperator(doneChan, &wg)
	if err != nil {
		log.Fatal(err)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-signalChan:
			log.Fatal("Received shutdown signal...")
			close(doneChan)
			wg.Wait()
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

func newAgent(kubeCfgFile string) (*Agent, error) {
	var (
		config *rest.Config
		err    error
	)
	if kubeCfgFile != "" {
		log.Printf("Using OutOfCluster k8s config with kubeConfigFile: %s", kubeCfgFile)
		config, err = clientcmd.BuildConfigFromFlags("", kubeCfgFile)
		if err != nil {
			panic(err.Error())
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	crdClient, err := clientset.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

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
		crdClient: crdClient,
		kclient:   kclient,
		kubeExt:   kubeExt,
	}, nil
}

func initOperator(doneChan chan struct{}, wg *sync.WaitGroup) error {
	agent, err := newAgent(kubeCfgFile)
	if err != nil {
		panic(err.Error())
	}

	err = createCRD(agent.kubeExt)
	if err != nil {
		return err
	}

	wg.Add(1)
	watchEchoEvents(doneChan, wg, agent.crdClient)

	return nil
}

func monitorEchoEvents(stopchan chan struct{}, crdClient clientset.Interface) (<-chan *tinyopv1.EchoServer, <-chan error) {
	events := make(chan *tinyopv1.EchoServer)
	errc := make(chan error, 1)

	source := cache.NewListWatchFromClient(crdClient.TinyopV1alpha1().RESTClient(), tinyop.ResourcePlural, v1.NamespaceAll, fields.Everything())

	createHandler := func(obj interface{}) {
		event := obj.(*tinyopv1.EchoServer)
		event.Type = "ADDED"
		events <- event
	}

	deleteHandler := func(obj interface{}) {
		event := obj.(*tinyopv1.EchoServer)
		event.Type = "DELETED"
		events <- event
	}

	updateHandler := func(old interface{}, obj interface{}) {
		event := obj.(*tinyopv1.EchoServer)
		event.Type = "MODIFIED"
		events <- event
	}

	_, controller := cache.NewInformer(
		source,
		&tinyopv1.EchoServer{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    createHandler,
			UpdateFunc: updateHandler,
			DeleteFunc: deleteHandler,
		})

	go controller.Run(stopchan)

	return events, errc
}

func watchEchoEvents(done chan struct{}, wg *sync.WaitGroup, crdClient clientset.Interface) {
	events, watchErrs := monitorEchoEvents(done, crdClient)
	go func() {
		for {
			select {
			case event := <-events:
				err := handleEchoEvent(event)
				if err != nil {
					log.Fatal(err)
				}
			case err := <-watchErrs:
				log.Fatal(err)
			case <-done:
				wg.Done()
				return
			}
		}
	}()
}

func createCRD(kubeExt apiextensionsclient.Interface) error {
	crd, err := kubeExt.ApiextensionsV1beta1().CustomResourceDefinitions().Get(tinyop.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			crdObject := &apiextensionsv1beta1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: tinyop.Name,
				},
				Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
					Group:   tinyop.GroupName,
					Version: tinyop.Version,
					Scope:   apiextensionsv1beta1.NamespaceScoped,
					Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
						Plural: tinyop.ResourcePlural,
						Kind:   tinyop.ResourceKind,
					},
				},
			}

			_, err := kubeExt.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crdObject)
			if err != nil {
				panic(err)
			}
			log.Print("Initialized CRD...waiting for K8s system to acknowledge...")

			// wait for CRD being established
			err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
				createdCRD, err := kubeExt.ApiextensionsV1beta1().CustomResourceDefinitions().Get(tinyop.Name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				for _, cond := range createdCRD.Status.Conditions {
					switch cond.Type {
					case apiextensionsv1beta1.Established:
						if cond.Status == apiextensionsv1beta1.ConditionTrue {
							return true, nil
						}
					case apiextensionsv1beta1.NamesAccepted:
						if cond.Status == apiextensionsv1beta1.ConditionFalse {
							return false, fmt.Errorf("Name conflict: %v", cond.Reason)
						}
					}
				}
				return false, nil
			})

			if err != nil {
				deleteErr := kubeExt.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(tinyop.Name, nil)
				if deleteErr != nil {
					return errors.NewAggregate([]error{err, deleteErr})
				}
				return err
			}

			log.Print("CRD is alive.")
		} else {
			panic(err)
		}
	} else {
		log.Printf("CRD already exists %#v\n", crd.ObjectMeta.Name)
	}

	return nil
}

func handleEchoEvent(e *tinyopv1.EchoServer) error {
	eLock.Lock()
	defer eLock.Unlock()
	switch {
	case e.Type == "ADDED" || e.Type == "MODIFIED":
		return onApplyEcho(e)
	case e.Type == "DELETED":
		return onDeleteEcho(e)
	}
	return nil
}

func onApplyEcho(e *tinyopv1.EchoServer) error {
	log.Printf("Received create event with object: %#v", e)
	return nil
}

func onDeleteEcho(e *tinyopv1.EchoServer) error {
	log.Printf("Received delete event with object: %#v", e)
	return nil
}
