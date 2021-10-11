/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package app

import (
	"fmt"
	"github.com/spf13/cobra"
	genericapiserver "k8s.io/apiserver/pkg/server"
	daemon "k8s.io/kubernetes/cmd/kubelite/app/daemons"
	"k8s.io/kubernetes/cmd/kubelite/app/options"
	"os"
	"time"
)

var opts = options.NewOptions()

// liteCmd represents the base command when called without any subcommands
var liteCmd = &cobra.Command{
	Use:   "kubelite",
	Short: "Single server kubernetes",
	Long: `A single server that spawns all other kubernetes servers as threads`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		ctx := genericapiserver.SetupSignalContext()

		if opts.StartControlPlane {
			apiserverArgs := options.ReadArgsFromFile(opts.APIServerArgsFile)
			go daemon.StartAPIServer(apiserverArgs, ctx.Done())
			daemon.WaitForAPIServer(opts.KubeconfigFile, 360 * time.Second)

			if opts.StartControllerManager {
				controllerArgs := options.ReadArgsFromFile(opts.ControllerManagerArgsFile)
				go daemon.StartControllerManager(controllerArgs, ctx)
			}

			if opts.StartScheduler {
				schedulerArgs := options.ReadArgsFromFile(opts.SchedulerArgsFile)
				go daemon.StartScheduler(schedulerArgs, ctx)
			}
		}

		proxyArgs := options.ReadArgsFromFile(opts.ProxyArgsFile)
		go daemon.StartProxy(proxyArgs)

		kubeletArgs := options.ReadArgsFromFile(opts.KubeletArgsFile)
		daemon.StartKubelet(kubeletArgs, ctx)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the liteCmd.
func Execute() {
	if err := liteCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize()

	liteCmd.Flags().StringVar(&opts.SchedulerArgsFile, "scheduler-args-file", opts.SchedulerArgsFile, "file with the arguments for the scheduler")
	liteCmd.Flags().BoolVar(&opts.StartScheduler, "start-scheduler", opts.StartScheduler, "start the scheduler")
	liteCmd.Flags().StringVar(&opts.ControllerManagerArgsFile, "controller-manager-args-file", opts.ControllerManagerArgsFile, "file with the arguments for the controller manager")
	liteCmd.Flags().BoolVar(&opts.StartControllerManager, "start-controller-manager", opts.StartControllerManager, "start the controller manager")
	liteCmd.Flags().StringVar(&opts.ProxyArgsFile, "proxy-args-file", opts.ProxyArgsFile , "file with the arguments for kube-proxy")
	liteCmd.Flags().StringVar(&opts.KubeletArgsFile, "kubelet-args-file", opts.KubeletArgsFile, "file with the arguments for kubelet")
	liteCmd.Flags().StringVar(&opts.APIServerArgsFile, "apiserver-args-file", opts.APIServerArgsFile, "file with the arguments for the API server")
	liteCmd.Flags().StringVar(&opts.KubeconfigFile , "kubeconfig-file", opts.KubeconfigFile, "the kubeconfig file to use to healthcheck the API server")
	liteCmd.Flags().BoolVar(&opts.StartControlPlane, "start-control-plane", opts.StartControlPlane, "start the control plane (API server, scheduler and controller manager)")
}
