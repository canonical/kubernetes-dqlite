/*
Copyright 2018 The Kubernetes Authors.

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

package options

import (
	"bufio"
	"k8s.io/klog/v2"
	"os"
	"strings"
)

// Options has all the params needed to run a Kubelite
type Options struct {
	SchedulerArgsFile         string
	ControllerManagerArgsFile string
	ProxyArgsFile             string
	KubeletArgsFile           string
	APIServerArgsFile         string
	KubeconfigFile    		  string
	StartControlPlane		  bool
}

func NewOptions() (*Options){
	o := Options{
		"/var/snap/microk8s/current/args/kube-scheduler",
		"/var/snap/microk8s/current/args/kube-controller-manager",
		"/var/snap/microk8s/current/args/kube-proxy",
		"/var/snap/microk8s/current/args/kubelet",
		"/var/snap/microk8s/current/args/kube-apiserver",
		"/var/snap/microk8s/current/credentials/client.config",
		true,
	}
	return &o
}

func ReadArgsFromFile(filename string) []string {
	var args []string
	file, err := os.Open(filename)
	if err != nil {
		klog.Fatalf("Failed to open arguments file %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		// ignore lines with # and empty lines
		if len(line) <= 0 || strings.HasPrefix(line, "#") {
			continue
		}
		// remove " and '
		for _, r := range "\"'" {
			line = strings.ReplaceAll(line, string(r), "")
		}
		for _, part := range strings.Split(line, " ") {

			args = append(args, os.ExpandEnv(part))
		}
	}
	if err := scanner.Err(); err != nil {
		klog.Fatalf("Failed to read arguments file %v", err)
	}
	return args
}
