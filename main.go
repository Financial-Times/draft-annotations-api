package main

import (
	"net/http"
	"os"

	"github.com/Financial-Times/api-endpoint"
	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/concept"
	"github.com/Financial-Times/draft-annotations-api/handler"
	"github.com/Financial-Times/draft-annotations-api/health"
	"github.com/Financial-Times/go-ft-http-transport/transport"
	"github.com/Financial-Times/http-handlers-go/httphandlers"
	status "github.com/Financial-Times/service-status-go/httphandlers"
	"github.com/husobee/vestigo"
	"github.com/jawher/mow.cli"
	"github.com/rcrowley/go-metrics"
	log "github.com/sirupsen/logrus"
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
		Desc:   "Endpoint to get concepts from UPP",
		EnvVar: "ANNOTATIONS_RW_ENDPOINT",
	})

	annotationsAPIEndpoint := app.String(cli.StringOpt{
		Name:   "upp-annotations-endpoint",
		Value:  "http://test.api.ft.com/content/%v/annotations",
		Desc:   "Endpoint to get annotations from UPP",
		EnvVar: "ANNOTATIONS_ENDPOINT",
	})

	internalConcordancesEndpoint := app.String(cli.StringOpt{
		Name:   "internal-concordances-endpoint",
		Value:  "http://test.api.ft.com/internalconcordances",
		Desc:   "Endpoint to get concepts from UPP",
		EnvVar: "INTERNAL_CONCORDANCES_ENDPOINT",
	})

	internalConcordancesBatchSize := app.Int(cli.IntOpt{
		Name:   "internal-concordances-batch-size",
		Value:  30,
		Desc:   "Concept IDs maximum batch size to use when querying the UPP Internal Concordances API",
		EnvVar: "INTERNAL_CONCORDANCES_BATCH_SIZE",
	})

	uppAPIKey := app.String(cli.StringOpt{
		Name:   "upp-api-key",
		Value:  "",
		Desc:   "API key to access UPP",
		EnvVar: "UPP_APIKEY",
	})

	apiYml := app.String(cli.StringOpt{
		Name:   "api-yml",
		Value:  "./api.yml",
		Desc:   "Location of the API Swagger YML file.",
		EnvVar: "API_YML",
	})

	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)
	log.Infof("[Startup] %v is starting", *appSystemCode)

	app.Action = func() {
		log.Infof("System code: %s, App Name: %s, Port: %s", *appSystemCode, *appName, *port)

		client := &http.Client{Transport: transport.NewTransport().WithStandardUserAgent("PAC", *appSystemCode)}

		rw := annotations.NewRW(client, *annotationsRWEndpoint)
		annotationsAPI := annotations.NewUPPAnnotationsAPI(client, *annotationsAPIEndpoint, *uppAPIKey)
		c14n := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
		conceptSearchAPI := concept.NewSearchAPI(client, *internalConcordancesEndpoint, *uppAPIKey, *internalConcordancesBatchSize)
		augmenter := annotations.NewAugmenter(conceptSearchAPI)
		annotationsHandler := handler.New(rw, annotationsAPI, c14n, augmenter)
		healthService := health.NewHealthService(*appSystemCode, *appName, appDescription, rw, annotationsAPI, conceptSearchAPI)

		serveEndpoints(*port, apiYml, annotationsHandler, healthService)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("App could not start, error=[%s]\n", err)
		return
	}
}

func serveEndpoints(port string, apiYml *string, handler *handler.Handler, healthService *health.HealthService) {
	r := vestigo.NewRouter()

	r.Get("/drafts/content/:uuid/annotations", handler.ReadAnnotations)
	r.Put("/drafts/content/:uuid/annotations", handler.WriteAnnotations)
	var monitoringRouter http.Handler = r
	monitoringRouter = httphandlers.TransactionAwareRequestLoggingHandler(log.StandardLogger(), monitoringRouter)
	monitoringRouter = httphandlers.HTTPMetricsHandler(metrics.DefaultRegistry, monitoringRouter)

	http.HandleFunc("/__health", healthService.HealthCheckHandleFunc())
	http.HandleFunc(status.GTGPath, status.NewGoodToGoHandler(healthService.GTG))
	http.HandleFunc(status.BuildInfoPath, status.BuildInfoHandler)

	http.Handle("/", monitoringRouter)

	if apiYml != nil {
		apiEndpoint, err := api.NewAPIEndpointForFile(*apiYml)
		if err != nil {
			log.WithError(err).WithField("file", *apiYml).Warn("Failed to serve the API Endpoint for this service. Please validate the Swagger YML and the file location")
		} else {
			r.Get(api.DefaultPath, apiEndpoint.ServeHTTP)
		}
	}

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Unable to start: %v", err)
	}
}
