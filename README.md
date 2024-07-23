# Draft Annotations API
[![Circle CI](https://circleci.com/gh/Financial-Times/draft-annotations-api/tree/master.png?style=shield)](https://circleci.com/gh/Financial-Times/draft-annotations-api/tree/master)[![Go Report Card](https://goreportcard.com/badge/github.com/Financial-Times/draft-annotations-api)](https://goreportcard.com/report/github.com/Financial-Times/draft-annotations-api) [![Coverage Status](https://coveralls.io/repos/github/Financial-Times/draft-annotations-api/badge.svg)](https://coveralls.io/github/Financial-Times/draft-annotations-api)

## Introduction

Draft Annotations API is a microservice that provides access to draft annotations for content stored in PAC.

## Installation

Download the source code, dependencies and test dependencies:

```
git clone https://github.com/Financial-Times/draft-annotations-api.git
cd draft-annotations-api
go build -mod=readonly
```

## Running locally

1. Run the tests and install the binary:

```
GO111MODULE=on go test -mod=readonly ./...
go install
```

2. Run the binary (using the `help` flag to see the available optional arguments):

```
$GOPATH/bin/draft-annotations-api [--help]

Options:
  --app-system-code="draft-annotations-api"                                        System Code of the application ($APP_SYSTEM_CODE)
  --app-name="draft-annotations-api"                                               Application name ($APP_NAME)
  --port="8080"                                                                    Port to listen on ($APP_PORT)
  --annotations-rw-endpoint="http://localhost:8888"                                Endpoint to get draft annotations from DB ($ANNOTATIONS_RW_ENDPOINT)
  --upp-annotations-endpoint="http://test.api.ft.com/content/%v/annotations"       Public Annotations API endpoint ($ANNOTATIONS_ENDPOINT)
  --internal-concordances-endpoint="http://test.api.ft.com/internalconcordances"   Endpoint to get concepts from UPP ($INTERNAL_CONCORDANCES_ENDPOINT)
  --internal-concordances-batch-size=30                                            Concept IDs maximum batch size to use when querying the UPP Internal Concordances API ($INTERNAL_CONCORDANCES_BATCH_SIZE)
  --upp-api-key=""                                                                 API key to access UPP ($UPP_APIKEY)
  --api-yml="./_ft/api.yml"                                                        Location of the API Swagger YML file. ($API_YML)
  --http-timeout="8s"                                                              Duration to wait before timing out a request ($HTTP_TIMEOUT)
  --log-level="INFO"                                                               Log level ($LOG_LEVEL)
```


3. Test:

    1. Either using curl:

            curl http://localhost:8080/draft-annotations/content/b7b871f6-8a89-11e4-8e24-00144feabdc0/annotations | json_pp

    1. Or using [httpie](https://github.com/jkbrzt/httpie):

            http GET http://localhost:8080/draft-annotations/content/b7b871f6-8a89-11e4-8e24-00144feabdc0/annotations

## Build and deployment

* The application is built as a Docker image inside a helm chart to be deployed in a Kubernetes cluster.
  An internal Jenkins job takes care to push the Docker image to Docker Hub and deploy the chart when a tag is created.
  This is the Docker Hub repository: [coco/draft-annotations-api](https://hub.docker.com/r/coco/draft-annotations-api)
* CI provided by CircleCI: [draft-annotations-api](https://circleci.com/gh/Financial-Times/draft-annotations-api)

## Service endpoints

For a full description of API endpoints for the service, please see the [Open API specification](./_ft/api.yml).

### GET - Reading draft annotations from PAC

Using curl:

```
curl http://localhost:8080/draft-annotations/content/{content-uuid}/annotations | jq
```

A GET request on this endpoint fetches the draft annotations for a specific piece of content by calling
[Generic RW Aurora](https://github.com/Financial-Times/generic-rw-aurora).
In case the success, annotations are enriched with concept information by calling
[UPP Concept Search API](https://github.com/Financial-Times/concept-search-api).
In case annotations are not available in PAC,
Draft Annotations API fetches published annotations by calling
[UPP Public Annotations API](https://github.com/Financial-Times/public-annotations-api).
Fetching published annotations is part of the strategy for dynamic importing legacy annotations in PAC.

This is an example response body:
```
{
      "annotations":[
      {
        "predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
        "id": "http://www.ft.com/thing/fd6734a1-3ae2-30f3-98a1-e373f8da8bf1",
        "apiUrl": "http://api.ft.com/people/fd6734a1-3ae2-30f3-98a1-e373f8da8bf1",
        "type": "http://www.ft.com/ontology/person/Person",
        "prefLabel": "Emily Cadman",
        "isFTAuthor": true,
      },
      {
        "predicate": "http://www.ft.com/ontology/annotation/hasContributor",
        "id": "http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
        "apiUrl": "http://api.ft.com/people/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
        "type": "http://www.ft.com/ontology/person/Person",
        "prefLabel": "Lisa Barrett",
        "isFTAuthor": true,
      },
      {
        "predicate": "http://www.ft.com/ontology/annotation/about",
        "id": "http://www.ft.com/thing/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
        "apiUrl": "http://api.ft.com/things/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
        "type": "http://www.ft.com/ontology/Topic",
        "prefLabel": "Global economic growth"
      }
    ]
}
```

### POST - Adding draft editorial annotations and writing them in PAC

Using curl:

```
curl http://localhost:8080/draft-annotations/content/{content-uuid}/annotations -X POST --data '{
          "id": "http://www.ft.com/thing/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd"
          "predicate": "http://www.ft.com/ontology/annotation/about",
}'
```

A POST request on this endpoint adds an annotation to the editorially curated published annotations for a specific piece of content. To retrieve these annotations it calls [UPP Public Annotations API](https://github.com/Financial-Times/public-annotations-api) using the "lifecycle" parameter.
The new list of draft annotations will override any unpublished draft annotations for this piece of content.
If the operation is successful, the application returns an HTTP 200 response code.

### PUT - Writing draft annotations to PAC

Using curl:
```
curl -X PUT \
  http://localhost:8080/draft-annotations/content/{content-uuid}/annotations \
  -d '{
            "annotations":[
            {
              "predicate": "http://www.ft.com/ontology/annotation/hasContributor",
              "id": "http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
              "apiUrl": "http://api.ft.com/people/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
              "type": "http://www.ft.com/ontology/person/Person",
              "prefLabel": "Lisa Barrett"
            },
            {
              "predicate": "http://www.ft.com/ontology/annotation/about",
              "id": "http://www.ft.com/thing/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
              "apiUrl": "http://api.ft.com/things/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
              "type": "http://www.ft.com/ontology/Topic",
              "prefLabel": "Global economic growth"
            },
            {
              "predicate": "http://www.ft.com/ontology/annotation/hasDisplayTag",
              "id": "http://www.ft.com/thing/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
              "apiUrl": "http://api.ft.com/things/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
              "type": "http://www.ft.com/ontology/Topic",
              "prefLabel": "Global economic growth"
            }
          ]
      }'
```

A PUT request on this endpoint writes the draft annotations in PAC.
The input body is an array of annotation JSON objects in which only `predicate` and `id` are the required fields.
If the write operation is successful, the application returns the canonicalized input body with
an HTTP 200 response code.
The listings below shows an example of a canonicalized response.

```
{
      "annotations":[
      {
        "predicate": "http://www.ft.com/ontology/annotation/hasContributor",
        "id": "http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
      },
      {
        "predicate": "http://www.ft.com/ontology/annotation/about",
        "id": "http://www.ft.com/thing/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
      },
      {
        "predicate": "http://www.ft.com/ontology/annotation/hasDisplayTag",
        "id": "http://www.ft.com/thing/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd",
      }
    ]
}
```

### DELETE - Deleting draft editorial annotations and writing them in PAC

Using curl:

```
curl http://localhost:8080/draft-annotations/content/{content-uuid}/annotations/{concept-uuid} | jq
```

A DELETE request on this endpoint deletes all the annotations for a single concept from the editorially curated published annotations for a specific piece of content. To retrieve these specific annotations it calls [UPP Public Annotations API](https://github.com/Financial-Times/public-annotations-api) using the "lifecycle" parameter.
If the operation is successful, the application returns an HTTP 200 response code.

### PATCH - Replacing draft editorial annotations

Using curl:

```
curl http://localhost:8080/draft-annotations/content/{content-uuid}/annotations/{concept-uuid} -X PATCH --data '{
        "id": "http://www.ft.com/thing/d7de27f8-1633-3fcc-b308-c95a2ad7d1cd"
}'
```

A PATCH request on this endpoint replaces all annotations for a single concept in the editorially curated published annotations for a specific piece of content. To retrieve these annotations it calls [UPP Public Annotations API](https://github.com/Financial-Times/public-annotations-api) using the "lifecycle" parameter.
The new list of draft annotations will override any unpublished draft annotations for this piece of content.
If the operation is successful, the application returns an HTTP 200 response code.

## Healthchecks

Admin endpoints are:

`/__gtg`
`/__health`
`/__build-info`

At the moment the `/__health` and `/__gtg` check the availability of the UPP Public Annotations API.

### Logging

* The application uses [FT Logger](https://github.com/Financial-Times/go-logger)
* Logs are written to the standard output.
* 
* NOTE: `/__build-info` and `/__gtg` endpoints are not logged as they are called every second from varnish/vulcand and this information is not needed in logs/splunk.


### Change/Rotate sealed secrets
Please refer to documentation in [pac-global-sealed-secrets-eks](https://github.com/Financial-Times/pac-global-sealed-secrets-eks/blob/master/README.md). Here are explained details how to create new and change existing sealed secrets