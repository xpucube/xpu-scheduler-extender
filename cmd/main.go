package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/controller"
	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/routes"
	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/scheduler"
	"github.com/YoYoContainerService/xpu-scheduler-extender/pkg/utils/signals"
	"github.com/comail/colog"
	"github.com/julienschmidt/httprouter"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const RecommendedKubeConfigPathEnv = "KUBECONFIG"

var (
	clientset    *kubernetes.Clientset
	resyncPeriod = 30 * time.Second
	clientConfig clientcmd.ClientConfig
)

func initKubeClient() {
	kubeConfig := ""
	if len(os.Getenv(RecommendedKubeConfigPathEnv)) > 0 {
		// use the current context in kubeconfig
		// This is very useful for running locally.
		kubeConfig = os.Getenv(RecommendedKubeConfigPathEnv)
	}

	// Get kubernetes config.
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		log.Fatalf("error building kubeconfig: %s", err.Error())
	}

	// create the clientset
	clientset, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("fatal: failed to init rest config due to %v", err)
	}
}

func main() {
	// Call Parse() to avoid noisy logs
	flag.CommandLine.Parse([]string{})

	colog.SetDefaultLevel(colog.LInfo)
	colog.SetMinLevel(colog.LInfo)
	colog.SetFormatter(&colog.StdFormatter{
		Colors: true,
		Flag:   log.Ldate | log.Ltime | log.Lshortfile,
	})
	colog.Register()
	level := StringToLevel(os.Getenv("LOG_LEVEL"))
	log.Print("log level was set to ", strings.ToUpper(level.String()))
	colog.SetMinLevel(level)
	threadness := StringToInt(os.Getenv("THREADNESS"))

	initKubeClient()
	port := os.Getenv("PORT")
	if _, err := strconv.Atoi(port); err != nil {
		port = "39999"
	}

	// Set up signals so we handle the first shutdown signal gracefully.
	stopCh := signals.SetupSignalHandler()

	informerFactory := kubeinformers.NewSharedInformerFactory(clientset, resyncPeriod)
	controller, err := controller.NewController(clientset, informerFactory, stopCh)
	if err != nil {
		log.Fatalf("failed to start due to %v", err)
	}
	err = controller.BuildCache()
	if err != nil {
		log.Fatalf("failed to start due to %v", err)
	}

	go controller.Run(threadness, stopCh)

	xpuPredicate := scheduler.NewXPUPredicate(clientset, controller.GetSchedulerCache())
	xpuBind := scheduler.NewXPUBind(clientset, controller.GetSchedulerCache())
	xpuInspect := scheduler.NewXPUInspect(controller.GetSchedulerCache())

	router := httprouter.New()

	routes.AddPProf(router)
	routes.AddVersion(router)
	routes.AddPredicate(router, xpuPredicate)
	routes.AddBind(router, xpuBind)
	routes.AddInspect(router, xpuInspect)

	log.Printf("info: server starting on the port :%s", port)
	if err := http.ListenAndServe(":" + port, router); err != nil {
		log.Fatal(err)
	}
}

func StringToLevel(levelStr string) colog.Level {
	switch level := strings.ToUpper(levelStr); level {
	case "TRACE":
		return colog.LTrace
	case "DEBUG":
		return colog.LDebug
	case "INFO":
		return colog.LInfo
	case "WARNING":
		return colog.LWarning
	case "ERROR":
		return colog.LError
	case "ALERT":
		return colog.LAlert
	default:
		log.Printf("warning: LOG_LEVEL=\"%s\" is empty or invalid, fallling back to \"INFO\".\n", level)
		return colog.LInfo
	}
}

func StringToInt(sThread string) int {
	thread := 1

	return thread
}
