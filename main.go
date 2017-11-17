package main

import (
	"net/http"
	"os"

	api "github.com/Financial-Times/api-endpoint"
	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/concept"
	"github.com/Financial-Times/draft-annotations-api/handler"
	"github.com/Financial-Times/draft-annotations-api/health"
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

	conceptSearchEndpoint := app.String(cli.StringOpt{
		Name:   "concept-search-endpoint",
		Value:  "http://test.api.ft.com/concepts",
		Desc:   "Endpoint to get concepts from UPP",
		EnvVar: "CONCEPT_SEARCH_ENDPOINT",
	})

	conceptSearchBatchSize := app.Int(cli.IntOpt{
		Name:   "concept-search-batch-size",
		Value:  30,
		Desc:   "Concept IDs batch size to concept search API",
		EnvVar: "CONCEPT_SEARCH_BATCH_SIZE",
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

		rw := annotations.NewRW(*annotationsRWEndpoint)
		annotationsAPI := annotations.NewUPPAnnotationsAPI(*annotationsAPIEndpoint, *uppAPIKey)
		c14n := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
		conceptSearchAPI := concept.NewSearchAPI(*conceptSearchEndpoint, *uppAPIKey, *conceptSearchBatchSize)
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
