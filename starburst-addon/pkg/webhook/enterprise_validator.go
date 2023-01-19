/*
Copyright 2023.

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

package webhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-logr/logr"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	ctrl "sigs.k8s.io/controller-runtime"
)

var EnterpriseGvk = schema.GroupVersionKind{
	Kind:    "StarburstEnterprise",
	Group:   "example.com.example.com",
	Version: "v1alpha1",
}

// StarburstEnterpriseValidator is in charge of starting up the web server and serving our router
type StarburstEnterpriseValidator struct {
	ctrl.Manager
	logr.Logger
	runtime.Decoder
}

// ErrorLoggerWrapper is used for relaying the server error logger messages to our own logger
type ErrorLoggerWrapper struct {
	logr.Logger
}

func (w *ErrorLoggerWrapper) Write(p []byte) (int, error) {
	w.Logger.Error(errors.New(string(p)), "")
	return len(p), nil
}

// our validator instance
var validator *StarburstEnterpriseValidator

// add our validator to a manager
func Add(m ctrl.Manager) error {
	validator = &StarburstEnterpriseValidator{
		m,
		m.GetLogger(),
		serializer.NewCodecFactory(m.GetScheme()).UniversalDecoder(),
	}
	return m.Add(validator)
}

// start up the web server once once invoked by the manager
func (v *StarburstEnterpriseValidator) Start(ctx context.Context) error {
	if Options.TlsCert == "" || Options.TlsKey == "" {
		err := fmt.Errorf("missing arguments, 'tls-cert' and 'tls-key' are required")
		v.Logger.Error(err, "can not start validator without a certificate")
		return err
	}

	if Options.Port < 0 {
		err := fmt.Errorf("wrong argument, 'port' must be a positive value")
		v.Logger.Error(err, "can not start validator without a port")
		return err
	}

	// load certificate from key pair
	cert, err := tls.LoadX509KeyPair(Options.TlsCert, Options.TlsKey)
	if err != nil {
		err := fmt.Errorf("certificate issue, failed to load certificate and key")
		v.Logger.Error(err, "can not run without a certificate")
		return err
	}

	// create router
	router := http.NewServeMux()
	router.HandleFunc("/validate-enterprise", v.validateEnterprise)

	// create server
	webhookServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", Options.Port),
		Handler: router,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
		ErrorLog: log.New(&ErrorLoggerWrapper{v.Logger}, "https", log.LstdFlags),
	}

	// run server
	if err := webhookServer.ListenAndServeTLS("", ""); err != nil {
		v.Logger.Error(err, "failed to start webhook server")
		return err
	}

	return nil
}

// function for handling all validation requests
func (v *StarburstEnterpriseValidator) validateEnterprise(writer http.ResponseWriter, request *http.Request) {
	if err := verifyRequest(request); err != nil {
		v.writeError(writer, err, "request not verified", 400)
		return
	}

	admissionRequest, err := parseRequest(v.Decoder, request)
	if err != nil {
		v.writeError(writer, err, "failed parsing request", 400)
		return
	}

	operation := admissionRequest.Request.Operation

	// create admission response for setting the review status
	admissionResponse := &admissionv1.AdmissionResponse{}
	admissionResponse.UID = admissionRequest.Request.UID
	admissionResponse.Allowed = isUserAllowed(admissionRequest.Request.UserInfo)

	if admissionResponse.Allowed {
		if operation == admissionv1.Create {
			// parsing is only required if we need to verify something with object before allowing its creation
			// if there's no such requirement, this entire branch can be removed
			_, err := parseEnterprise(v.Decoder, admissionRequest.Request.Object.Raw)
			if err != nil {
				v.writeError(writer, err, "failed parsing object", 500)
				return
			}

			// NOTE add creation validation logic here if needed
		}

		if operation == admissionv1.Update {
			// updating is prohibited (can be modified based on requirements)
			admissionResponse.Allowed = false
		}

		if operation == admissionv1.Delete {
			// parsing is only required if we need to verify something with object before allowing its deletion
			// if there's no such requirement, this entire branch can be removed
			_, err := parseEnterprise(v.Decoder, admissionRequest.Request.OldObject.Raw)
			if err != nil {
				v.writeError(writer, err, "failed parsing object", 500)
				return
			}

			// NOTE add deletion validation logic here if needed
		}
	}

	if !admissionResponse.Allowed {
		// result is only read when not allowed
		admissionResponse.Result = &v1.Status{
			Status:  "Failure",
			Message: fmt.Sprintf("%s not allowed", operation),
			Code:    403,
			Reason:  "Unauthorized Access",
		}
	}

	// create admission review for responding
	admissionReview := &admissionv1.AdmissionReview{
		Response: admissionResponse,
	}
	admissionReview.SetGroupVersionKind(admissionRequest.GroupVersionKind())

	// serialize the response
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		v.writeError(writer, err, "failed creating response", 500)
		return
	}

	// stream back http response
	writer.Header().Set("Content-Type", "application/json")
	writer.Write(resp)
}

// write error message to log and stream back as an http response
func (v *StarburstEnterpriseValidator) writeError(writer http.ResponseWriter, err error, msg string, code int) {
	v.Logger.Error(err, msg)
	writer.WriteHeader(code)
	writer.Write([]byte(msg))
}

// verify the request is of type application/json and has a legit json body
func verifyRequest(request *http.Request) error {
	if request.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("expected application/json content-type")
	}

	if request.Body == nil {
		return fmt.Errorf("no request body found")
	}

	// is this an overkill?
	// if _, err := json.Marshal(request.Body); err != nil {
	// 	return fmt.Errorf("failed parsing body")
	// }

	return nil
}

// use the decoder for parsing the http request into an admission review request
func parseRequest(decoder runtime.Decoder, request *http.Request) (*admissionv1.AdmissionReview, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	admissionReviewRequest := &admissionv1.AdmissionReview{}
	if _, _, err := decoder.Decode(body, nil, admissionReviewRequest); err != nil {
		return nil, err
	}

	return admissionReviewRequest, nil
}

// use the decoder to parsing bytes into an unstructured starburst enterprise
func parseEnterprise(decoder runtime.Decoder, rawObj []byte) (unstructured.Unstructured, error) {
	enterprise := unstructured.Unstructured{}
	enterprise.SetGroupVersionKind(EnterpriseGvk)

	if _, _, err := decoder.Decode(rawObj, nil, &enterprise); err != nil {
		return enterprise, err
	}
	return enterprise, nil
}

// verify the requesting use is our own service account
func isUserAllowed(userInfo authenticationv1.UserInfo) bool {
	infoArray := strings.Split(userInfo.Username, ":")

	return infoArray[0] == "system" &&
		infoArray[1] == "serviceaccount" &&
		infoArray[3] == "starburst-addon-service-account"
}
