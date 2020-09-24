package daemon

import (
	"context"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	genericcontrollermanager "k8s.io/controller-manager/app"
	apiserver "k8s.io/kubernetes/cmd/kube-apiserver/app"
	controller "k8s.io/kubernetes/cmd/kube-controller-manager/app"
	proxy "k8s.io/kubernetes/cmd/kube-proxy/app"
	scheduler "k8s.io/kubernetes/cmd/kube-scheduler/app"
	kubelet "k8s.io/kubernetes/cmd/kubelet/app"

	"time"
)

func StartControllerManager(args []string, ctx context.Context) {
	command := controller.NewControllerManagerCommand()
	command.SetArgs(args)

	klog.Info("Starting Controller Manager")
	if err := command.ExecuteContext(ctx); err != nil {
		klog.Fatalf("Controller Manager exited %v", err)
	}
	klog.Info("Stopping Controller Manager")
}

func StartScheduler(args []string, ctx context.Context) {
	command := scheduler.NewSchedulerCommand()
	command.SetArgs(args)

	klog.Info("Starting Scheduler")
	if err := command.ExecuteContext(ctx); err != nil {
		klog.Fatalf("Scheduler exited %v", err)
	}
	klog.Info("Stopping Scheduler")
}

func StartProxy(args []string) {
	command := proxy.NewProxyCommand()
	command.SetArgs(args)

	klog.Info("Starting Proxy")
	if err := command.Execute(); err != nil {
		klog.Fatalf("Proxy exited %v", err)
	}
	klog.Info("Stopping Proxy")
}

func StartKubelet(args []string, ctx context.Context) {
	command := kubelet.NewKubeletCommand(ctx)
	command.SetArgs(args)

	klog.Info("Starting Kubelet")
	if err := command.Execute(); err != nil {
		klog.Fatalf("Kubelet exited %v", err)
	}
	klog.Info("Stopping Kubelet")
}

func StartAPIServer(args []string, ctx <-chan struct{}) {
	command := apiserver.NewAPIServerCommand(ctx)
	command.SetArgs(args)
	klog.Info("Starting API Server")
	if err := command.Execute(); err != nil {
		klog.Fatalf("API Server exited %v", err)
	}
	klog.Info("Stopping API Server")
}

func WaitForAPIServer(kubeconfigpath string, timeout time.Duration) {
	klog.Info("Waiting for the API server")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigpath)
	if err != nil {
		klog.Fatalf("could not find the cluster's kubeconfig file %v", err)
	}
	// create the client
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("could not create client to the cluster %v", err)
	}
	genericcontrollermanager.WaitForAPIServer(client, timeout)
}