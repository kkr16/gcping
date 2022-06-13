// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	//"github.com/GoogleCloudPlatform/gcping/internal/config"
	"google.golang.org/api/run/v1"
)

var once sync.Once

type Service struct {
	URL        string
	Region     string
	RegionName string
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Serving on :%s", port)

	var AllEndpoints map[string]Service = generateConfig()

	region := os.Getenv("REGION")
	if region == "" {
		region = "pong"
	}

	// Serve / from files in kodata.
	kdp := os.Getenv("KO_DATA_PATH")
	if kdp == "" {
		log.Println("KO_DATA_PATH unset")
		kdp = "/var/run/ko/"
	}
	http.Handle("/", http.FileServer(http.Dir(kdp)))

	http.HandleFunc("/api/endpoints", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-store")
		w.Header().Add("Content-Type", "application/json;charset=utf-8")
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Strict-Transport-Security", "max-age=3600; includeSubdomains; preload")
		err := json.NewEncoder(w).Encode(AllEndpoints)
		if err != nil {
			w.WriteHeader(500)
		}
	})

	// Serve /api/ping with region response.
	http.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-store")
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Strict-Transport-Security", "max-age=3600; includeSubdomains; preload")
		once.Do(func() {
			w.Header().Add("X-First-Request", "true")
		})
		fmt.Fprintln(w, region)
	})

	// Serve /ping with region response to fix issue#96 on older cli versions.
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-store")
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Strict-Transport-Security", "max-age=3600; includeSubdomains; preload")
		once.Do(func() {
			w.Header().Add("X-First-Request", "true")
		})
		fmt.Fprintln(w, region)
	})

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func generateConfig() map[string]Service {
	log.Print("Using Cloud Run Admin API to generate Endpoints config.")
	ctx := context.Background()
	runService, err := run.NewService(ctx)
	// TODO: Get project name from Cloud Run metadata service if not defined in env variable
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")

	projresp, err := http.Get("http://metadata.google.internal/computeMetadata/v1/project/project-id")

	log.Print(*projresp)

	if err != nil {
		panic(err)
	}
	// List Services
	resp, err := runService.Namespaces.Services.List("namespaces/" + projectID).Fields("items(status/address/url,metadata(labels,name),spec(template/metadata/annotations))").LabelSelector("env=prod").Do()

	if err != nil {
		panic(err)
	}

	s, _ := json.MarshalIndent(resp.Items, "", "\t")

	var services []Service

	json.Unmarshal([]byte(s), &services)

	var ServicesMap = make(map[string]Service)

	var Global Service
	Global.Region = "global"
	Global.RegionName = "Global External HTTPS Load Balancer"
	Global.URL = "https://global.gcping.com"
	// TODO: Make Global.URL dynamic

	ServicesMap[Global.Region] = Global

	for _, s := range services {
		ServicesMap[s.Region] = s
		// or just keys, without values: elementMap[s] = ""
	}

	return ServicesMap
	//	output, _ := json.MarshalIndent(ServicesMap, "", "\t")

	//	fmt.Fprint(w, string(output))

}

func (es *Service) UnmarshalJSON(data []byte) error {
	// define private models for the data format

	type labelsInner struct {
		Location string `json:"cloud.googleapis.com/location"`
	}

	type annotationsInner struct {
		RegionName string `json:"region-name"`
	}

	type templateMetadataInner struct {
		Annotations annotationsInner `json:"annotations`
	}

	type templateInner struct {
		Metadata templateMetadataInner `json:"metadata"`
	}

	type specInner struct {
		Template templateInner `json:"template"`
	}

	type metadataInner struct {
		Labels labelsInner `json:"labels"`
		Name   string      `json:"name"`
	}

	type addressInner struct {
		Url string `json:"url"`
	}

	type statusInner struct {
		Address addressInner `json:"address"`
	}

	type nestedService struct {
		Metadata metadataInner `json:"metadata"`
		Status   statusInner   `json:"status"`
		Spec     specInner     `json:"spec"`
	}

	var ns nestedService

	if err := json.Unmarshal(data, &ns); err != nil {
		return err
	}

	// create the struct in desired format
	tmp := &Service{
		URL:        ns.Status.Address.Url,
		Region:     ns.Metadata.Labels.Location,
		RegionName: ns.Spec.Template.Metadata.Annotations.RegionName,
	}

	// reassign the method receiver pointer
	// to store the values in the struct
	*es = *tmp
	return nil
}
