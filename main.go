package main

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Financial-Times/cm-annotations-ontology/validator"
	"github.com/Financial-Times/draft-annotations-api/policy"
	"github.com/Financial-Times/go-logger/v2"
	"github.com/Financial-Times/opa-client-go"
	"github.com/Financial-Times/upp-content-validator-kit/v3/schema"
	"github.com/gorilla/mux"
	"github.com/rcrowley/go-metrics"

	apiEndpoint "github.com/Financial-Times/api-endpoint"
	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/concept"
	"github.com/Financial-Times/draft-annotations-api/handler"
	"github.com/Financial-Times/draft-annotations-api/health"
	"github.com/Financial-Times/go-ft-http/fthttp"
	"github.com/Financial-Times/http-handlers-go/v2/httphandlers"
	status "github.com/Financial-Times/service-status-go/httphandlers"
	cli "github.com/jawher/mow.cli"
)

const appDescription = "PAC Draft Annotations API"

func main() {
	app := cli.App("draft-annotations-api", appDescription)

	appSystemCode := app.String(cli.StringOpt{
		Name:   "app-system-code",
		Value:  "draft-annotations-api",
		Desc:   "System Code of the application",
		EnvVar: "APP_SYSTEM_CODE",
	})
	appName := app.String(cli.StringOpt{
		Name:   "app-name",
		Value:  "draft-annotations-api",
		Desc:   "Application name",
		EnvVar: "APP_NAME",
	})
	port := app.String(cli.StringOpt{
		Name:   "port",
		Value:  "8080",
		Desc:   "Port to listen on",
		EnvVar: "APP_PORT",
	})
	annotationsRWEndpoint := app.String(cli.StringOpt{
		Name:   "annotations-rw-endpoint",
		Value:  "http://localhost:8888",
		Desc:   "Endpoint to get draft annotations from DB",
		EnvVar: "ANNOTATIONS_RW_ENDPOINT",
	})
	annotationsAPIEndpoint := app.String(cli.StringOpt{
		Name:   "upp-annotations-endpoint",
		Value:  "https://upp-staging-delivery-glb.upp.ft.com/content/%v/annotations",
		Desc:   "Public Annotations API endpoint",
		EnvVar: "ANNOTATIONS_ENDPOINT",
	})
	internalConcordancesEndpoint := app.String(cli.StringOpt{
		Name:   "internal-concordances-endpoint",
		Value:  "https://upp-staging-delivery-glb.upp.ft.com/internalconcordances",
		Desc:   "Endpoint to get concepts from UPP",
		EnvVar: "INTERNAL_CONCORDANCES_ENDPOINT",
	})
	internalConcordancesBatchSize := app.Int(cli.IntOpt{
		Name:   "internal-concordances-batch-size",
		Value:  30,
		Desc:   "Concept IDs maximum batch size to use when querying the UPP Internal Concordances API",
		EnvVar: "INTERNAL_CONCORDANCES_BATCH_SIZE",
	})
	deliveryBasicAuth := app.String(cli.StringOpt{
		Name:   "delivery-basic-auth",
		Value:  "username:password",
		Desc:   "Basic auth for access to the delivery UPP clusters",
		EnvVar: "DELIVERY_BASIC_AUTH",
	})
	apiYml := app.String(cli.StringOpt{
		Name:   "api-yml",
		Value:  "./_ft/api.yml",
		Desc:   "Location of the API Swagger YML file.",
		EnvVar: "API_YML",
	})
	httpTimeoutDuration := app.String(cli.StringOpt{
		Name:   "http-timeout",
		Value:  "8s",
		Desc:   "Duration to wait before timing out a request",
		EnvVar: "HTTP_TIMEOUT",
	})
	logLevel := app.String(cli.StringOpt{
		Name:   "log-level",
		Value:  "INFO",
		Desc:   "Log level",
		EnvVar: "LOG_LEVEL",
	})
	OPAAddress := app.String(cli.StringOpt{
		Name:   "OPAAddress",
		Desc:   "Open policy agent sidecar address",
		Value:  "http://localhost:8181",
		EnvVar: "OPA_ADDRESS",
	})

	log := logger.NewUPPLogger(*appName, *logLevel)

	app.Action = func() {
		log.Infof("Starting with system code: %s, app name: %s, port: %s", *appSystemCode, *appName, *port)

		httpTimeout, err := time.ParseDuration(*httpTimeoutDuration)
		if err != nil {
			log.WithError(err).Fatal("Please provide a valid timeout duration")
		}

		client := fthttp.NewClientWithDefaultTimeout("PAC", *appSystemCode)

		basicAuthCredentials := strings.Split(*deliveryBasicAuth, ":")
		if len(basicAuthCredentials) != 2 {
			log.Fatal("error while resolving basic auth")
		}

		rw := annotations.NewRW(client, *annotationsRWEndpoint, log)
		annotationsAPI := annotations.NewUPPAnnotationsAPI(client, *annotationsAPIEndpoint, basicAuthCredentials[0], basicAuthCredentials[1], log)
		c14n := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
		conceptRead := concept.NewReadAPI(client, *internalConcordancesEndpoint, basicAuthCredentials[0], basicAuthCredentials[1], *internalConcordancesBatchSize, log)
		augmenter := annotations.NewAugmenter(conceptRead, log)
		schemaValidator := validator.NewSchemaValidator(log)
		jsonValidator := schemaValidator.GetJSONValidator()
		schemaHandler := schemaValidator.GetSchemaHandler()
		annotationsHandler := handler.New(rw, annotationsAPI, c14n, augmenter, jsonValidator, time.Millisecond*httpTimeout, log)
		healthService := health.NewHealthService(*appSystemCode, *appName, appDescription, rw, annotationsAPI, conceptRead)

		paths := map[string]string{
			policy.Key: policy.OpaPolicyPath,
		}

		opaClient := opa.NewOpenPolicyAgentClient(*OPAAddress, paths)

		serveEndpoints(*port, *apiYml, annotationsHandler, healthService, schemaHandler, log, opaClient)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("App could not start, error=[%s]\n", err)
		return
	}
}

func serveEndpoints(port string, apiYml string, handler *handler.Handler, healthService *health.HealthService, schemaHandler *schema.SchemasHandler, log *logger.UPPLogger, opaClient *opa.OpenPolicyAgentClient) {
	r := mux.NewRouter()

	middlewareFunc := opa.CreateRequestMiddleware(opaClient, policy.Key, log, policy.Middleware)

	authorizedRoutes := r.NewRoute().Subrouter()
	authorizedRoutes.Use(middlewareFunc)
	authorizedRoutes.HandleFunc("/draft-annotations/content/{uuid}/annotations", handler.ReadAnnotations).Methods(http.MethodGet)

	authorizedRoutes.HandleFunc("/draft-annotations/content/{uuid}/annotations", handler.WriteAnnotations).Methods(http.MethodPut)
	authorizedRoutes.HandleFunc("/draft-annotations/content/{uuid}/annotations", handler.AddAnnotation).Methods(http.MethodPost)
	authorizedRoutes.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", handler.ReplaceAnnotation).Methods(http.MethodPatch)

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", handler.DeleteAnnotation).Methods(http.MethodDelete)
	r.HandleFunc("/draft-annotations/validate", handler.Validate).Methods(http.MethodPost)
	r.HandleFunc("/draft-annotations/schemas", schemaHandler.ListSchemas).Methods(http.MethodGet)
	r.HandleFunc("/draft-annotations/schemas/{schemaName}", schemaHandler.GetSchema).Methods(http.MethodGet)

	if apiYml != "" {
		if endpoint, err := apiEndpoint.NewAPIEndpointForFile(apiYml); err == nil {
			r.HandleFunc(apiEndpoint.DefaultPath, endpoint.ServeHTTP).Methods("GET")
		}
	}

	http.HandleFunc("/__health", healthService.HealthCheckHandleFunc())
	http.HandleFunc(status.GTGPath, status.NewGoodToGoHandler(healthService.GTG))
	http.HandleFunc(status.BuildInfoPath, status.BuildInfoHandler)

	var monitoringRouter http.Handler = r
	monitoringRouter = httphandlers.TransactionAwareRequestLoggingHandler(log, monitoringRouter)
	monitoringRouter = httphandlers.HTTPMetricsHandler(metrics.DefaultRegistry, monitoringRouter)

	http.Handle("/", monitoringRouter)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Unable to start: %v", err)
	}
}
