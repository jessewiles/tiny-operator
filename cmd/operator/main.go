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

	"k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	appVersion  = "0.0.1"
	kubeCfgFile string
	eLock       = &sync.Mutex{}
)

func init() {
	flag.StringVar(&kubeCfgFile, "kubecfg-file", "", "location of kubecfg file")
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
	watchEchoEvents(doneChan, wg, agent)

	wg.Add(1)
	watchPodEvents(doneChan, wg, agent)

	return nil
}

// Echo server object event stuff
// monitorEchoEvents
func monitorEchoEvents(stopchan chan struct{}, a *Agent) (<-chan *tinyopv1.EchoServer, <-chan error) {
	events := make(chan *tinyopv1.EchoServer)
	errc := make(chan error, 1)

	source := cache.NewListWatchFromClient(a.crdClient.TinyopV1alpha1().RESTClient(), tinyop.ResourcePlural, v1.NamespaceAll, fields.Everything())

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

func watchEchoEvents(done chan struct{}, wg *sync.WaitGroup, a *Agent) {
	events, watchErrs := monitorEchoEvents(done, a)
	go func() {
		for {
			select {
			case event := <-events:
				err := handleEchoEvent(event, a)
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

func handleEchoEvent(e *tinyopv1.EchoServer, a *Agent) error {
	eLock.Lock()
	defer eLock.Unlock()
	switch {
	case e.Type == "ADDED" || e.Type == "MODIFIED":
		return onApplyEcho(e, a)
	case e.Type == "DELETED":
		return onDeleteEcho(e, a)
	}
	return nil
}

func onApplyEcho(e *tinyopv1.EchoServer, a *Agent) error {
	log.Printf("Received create event with object: %#v", e)
	makeNodeTypeService(e.ObjectMeta.Name, e.ObjectMeta.Namespace, "http", a)
	return doApplyEchoState(e, a)
}

func onDeleteEcho(e *tinyopv1.EchoServer, a *Agent) error {
	log.Printf("Received delete event with object: %#v", e)
	return nil
}

// Pod event stuff
func monitorPods(stopchan chan struct{}, a *Agent) (<-chan *v1.Pod, <-chan error) {
	events := make(chan *v1.Pod)
	errc := make(chan error, 1)

	podListWatcher := cache.NewListWatchFromClient(a.kclient.Core().RESTClient(), "pods", v1.NamespaceAll, fields.Everything())

	createAddHandler := func(obj interface{}) {
		event := obj.(*v1.Pod)
		for k, v := range event.ObjectMeta.Labels {
			if k == "component" && v == "echo-server" {
				event.ObjectMeta.Labels["es-event"] = "CREATE"
				events <- event
				break
			}
		}
	}

	updateHandler := func(old interface{}, obj interface{}) {
		event := obj.(*v1.Pod)
		for k, v := range event.ObjectMeta.Labels {
			if k == "component" && v == "echo-server" {
				event.ObjectMeta.Labels["es-event"] = "UPDATE"
				events <- event
				break
			}
		}
	}

	deleteHandler := func(obj interface{}) {
		event := obj.(*v1.Pod)
		for k, v := range event.ObjectMeta.Labels {
			if k == "component" && v == "echo-server" {
				event.ObjectMeta.Labels["es-event"] = "DELETE"
				events <- event
				break
			}
		}
	}

	_, controller := cache.NewIndexerInformer(podListWatcher, &v1.Pod{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    createAddHandler,
		UpdateFunc: updateHandler,
		DeleteFunc: deleteHandler,
	}, cache.Indexers{})

	go controller.Run(stopchan)

	return events, errc
}

func watchPodEvents(done chan struct{}, wg *sync.WaitGroup, a *Agent) {
	events, watchErrs := monitorPods(done, a)
	go func() {
		for {
			select {
			case event := <-events:
				err := handlePodEvent(event, a)
				if err != nil {
					log.Fatal(err)
				}
			case err := <-watchErrs:
				log.Fatal(err)
			case <-done:
				wg.Done()
				log.Print("Stopped pod event watcher.")
				return
			}
		}
	}()
}

func handlePodEvent(p *v1.Pod, a *Agent) error {
	eLock.Lock()
	defer eLock.Unlock()
	switch {
	case true:
		return onApplyPod(p, a)
	}
	return nil
}

func onApplyPod(p *v1.Pod, a *Agent) error {
	log.Printf("Received pod apply event with object: %#v", p.ObjectMeta.Labels)
	return nil
}

// Other functions
func createCRD(kubeExt apiextensionsclient.Interface) error {
	crd, err := kubeExt.ApiextensionsV1beta1().CustomResourceDefinitions().Get(tinyop.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			crdObject := &apiextensionsv1beta1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: tinyop.Name},
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

func doApplyEchoState(e *tinyopv1.EchoServer, a *Agent) error {
	deployment, err := a.kclient.ExtensionsV1beta1().Deployments(e.ObjectMeta.Namespace).Get(e.Name, metav1.GetOptions{})

	if len(deployment.Name) == 0 {
		log.Printf("Deployment %s not found, creating...", e.ObjectMeta.Name)

		deployment := &v1beta1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: e.ObjectMeta.Name, Labels: map[string]string{"component": "echo-server", "role": "es-replica", "name": e.ObjectMeta.Name}},
			Spec: v1beta1.DeploymentSpec{
				Replicas: &e.Spec.Replicas,
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"component": "echo-server", "role": "es-replica", "name": e.ObjectMeta.Name},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							v1.Container{
								Name:            e.ObjectMeta.Name,
								SecurityContext: &v1.SecurityContext{Privileged: &[]bool{true}[0], Capabilities: &v1.Capabilities{Add: []v1.Capability{"IPC_LOCK"}}},
								Image:           e.Spec.Image,
								ImagePullPolicy: "IfNotPresent",
								Env: []v1.EnvVar{
									v1.EnvVar{
										Name:  "CHARLIE_BROWN",
										Value: "Peanuts",
									},
								},
								Ports: []v1.ContainerPort{
									{
										Name:          "echo-srv-port",
										ContainerPort: 8200,
										Protocol:      v1.ProtocolTCP,
									},
								},
							},
						},
						ImagePullSecrets: []v1.LocalObjectReference{
							{Name: e.Spec.ImagePullSecretName},
						},
					},
				},
			},
		}

		_, err := a.kclient.AppsV1beta1().Deployments(e.ObjectMeta.Namespace).Create(deployment)

		if err != nil {
			log.Printf("Could not create echo server deployment: ", err)
			return err
		}
	} else {
		if err != nil {
			log.Printf("Count not get echo server deployment: ", err)
			return err
		}
		//scale replicas?
		if deployment.Spec.Replicas != &e.Spec.Replicas {
			deployment.Spec.Replicas = &e.Spec.Replicas

			if _, err := a.kclient.ExtensionsV1beta1().Deployments(e.ObjectMeta.Namespace).Update(deployment); err != nil {
				log.Printf("Could not scale deployment: ", err)
				return err
			}
		}
	}
	return nil
}

func makeNodeTypeService(clusterName, namespace, role string, a *Agent) error {
	fullClientServiceName := fmt.Sprintf("%s-%s", clusterName, namespace)
	_, err := a.kclient.CoreV1().Services(namespace).Get(fullClientServiceName, metav1.GetOptions{})

	if apierrors.IsNotFound(err) {
		log.Print("%s not found, creating echo server service...")

		clientSvc := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:   fullClientServiceName,
				Labels: map[string]string{"component": "es-service", "role": role},
			},
			Spec: v1.ServiceSpec{
				Type:     v1.ServiceTypeNodePort,
				Selector: map[string]string{"component": "es-service", "role": role},
				Ports: []v1.ServicePort{
					{
						Name:       "http",
						Port:       8000,
						Protocol:   "TCP",
						TargetPort: intstr.FromInt(8000),
						NodePort:   30800,
					},
				},
			},
		}

		if _, err := a.kclient.CoreV1().Services(namespace).Create(clientSvc); err != nil {
			log.Printf("Could not create node service: ", err)
			return err
		}

	} else if err != nil {
		log.Printf("Could not get node service: ", err)
		return err
	}

	return nil
}
