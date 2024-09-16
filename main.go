package main

import (
	"net/http"
	"os"
	"strings"
	"time"

	api "github.com/Financial-Times/api-endpoint"
	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/concept"
	"github.com/Financial-Times/draft-annotations-api/handler"
	"github.com/Financial-Times/draft-annotations-api/health"
	"github.com/Financial-Times/draft-annotations-api/synchronise"
	"github.com/Financial-Times/go-ft-http/fthttp"
	"github.com/Financial-Times/http-handlers-go/httphandlers"
	status "github.com/Financial-Times/service-status-go/httphandlers"
	"github.com/husobee/vestigo"
	cli "github.com/jawher/mow.cli"
	metrics "github.com/rcrowley/go-metrics"
	log "github.com/sirupsen/logrus"
)

const (
	platform       = "PAC"
	appDescription = "PAC Draft Annotations API"
)

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

	draftAnnotationsPublishEndpoint := app.String(cli.StringOpt{
		Name:   "draft-annotations-publish-endpoint",
		Value:  "http://localhost:8081",
		Desc:   "Endpoint to sync requests between pac and publish cluster",
		EnvVar: "DRAFT_ANNOTATIONS_PUBLISH_ENDPOINT",
	})

	publishBasicAuth := app.String(cli.StringOpt{
		Name:   "publish-basic-auth",
		Value:  "username:password",
		Desc:   "Basic auth for access to the publish UPP clusters",
		EnvVar: "PUBLISH_BASIC_AUTH",
	})

	log.SetFormatter(&log.JSONFormatter{})
	log.Infof("[Startup] %v is starting", *appSystemCode)

	app.Action = func() {
		log.Infof("System code: %s, App Name: %s, Port: %s", *appSystemCode, *appName, *port)

		// Setting the real log level here in order to have the startup log
		parsedLogLevel, err := log.ParseLevel(*logLevel)
		if err != nil {
			log.WithField("logLevel", *logLevel).WithError(err).Error("Incorrect log level. Using INFO instead.")
			parsedLogLevel = log.InfoLevel
		}
		log.SetLevel(parsedLogLevel)

		httpTimeout, err := time.ParseDuration(*httpTimeoutDuration)
		if err != nil {
			log.WithError(err).Fatal("Please provide a valid timeout duration")
		}

		client := fthttp.NewClientWithDefaultTimeout(platform, *appSystemCode)

		basicAuthCredentials := strings.Split(*deliveryBasicAuth, ":")
		if len(basicAuthCredentials) != 2 {
			log.Fatal("error while resolving basic auth")
		}

		publishBasicAuthCredentials := strings.Split(*publishBasicAuth, ":")
		if len(publishBasicAuthCredentials) != 2 {
			log.Fatal("error while resolving publish basic auth")
		}

		rw := annotations.NewRW(client, *annotationsRWEndpoint)
		annotationsAPI := annotations.NewUPPAnnotationsAPI(client, *annotationsAPIEndpoint, basicAuthCredentials[0], basicAuthCredentials[1])
		c14n := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
		conceptRead := concept.NewReadAPI(client, *internalConcordancesEndpoint, basicAuthCredentials[0], basicAuthCredentials[1], *internalConcordancesBatchSize)
		augmenter := annotations.NewAugmenter(conceptRead)
		annotationsHandler := handler.New(rw, annotationsAPI, c14n, augmenter, time.Millisecond*httpTimeout)
		healthService := health.NewHealthService(*appSystemCode, *appName, appDescription, rw, annotationsAPI, conceptRead)
		syncAPI := synchronise.NewAPI(client, publishBasicAuthCredentials[0], publishBasicAuthCredentials[1], *draftAnnotationsPublishEndpoint)
		serveEndpoints(*port, apiYml, annotationsHandler, healthService, syncAPI)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("App could not start, error=[%s]\n", err)
		return
	}
}

func serveEndpoints(port string, apiYml *string, handler *handler.Handler, healthService *health.HealthService, sapi *synchronise.API) {
	r := vestigo.NewRouter()

	requestMiddleware := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			err := sapi.SyncWithPublishingCluster(r)
			if err != nil {
				log.WithError(err).Info("error while sending request to publishing cluster")
			}
			// before the request
			f(w, r)
			// after the request
		}
	}

	r.Delete("/drafts/content/:uuid/annotations/:cuuid", handler.DeleteAnnotation, requestMiddleware)
	r.Get("/drafts/content/:uuid/annotations", handler.ReadAnnotations)
	r.Put("/drafts/content/:uuid/annotations", handler.WriteAnnotations, requestMiddleware)
	r.Post("/drafts/content/:uuid/annotations", handler.AddAnnotation, requestMiddleware)
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", handler.ReplaceAnnotation, requestMiddleware)

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
