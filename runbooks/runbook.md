<!--
    Written in the format prescribed by https://github.com/Financial-Times/runbook.md.
    Any future edits should abide by this format.
-->
# PAC Draft Annotations API

Draft Annotations API is a micro-service that provides create, read, update and delete endpoints for draft annotations stored in PAC.

## Code

draft-annotations-api

## Primary URL

https://pac-prod-glb.upp.ft.com/__draft-annotations-api

## Service Tier

Platinum

## Lifecycle Stage

Production

## Host Platform

AWS

## Architecture

Draft Annotations API is part of the PAC clusters, it is deployed in both EU and US regions with two replicas per deployment. The service uses the annotations published in UPP in order to produce draft annotations that are stored in PAC Aurora DB.

[PAC architecture diagram](https://app.lucidchart.com/publicSegments/view/22c1656b-6242-4da6-9dfb-f7225c20f38f/image.png)

## Contains Personal Data

No

## Contains Sensitive Data

No

<!-- Placeholder - remove HTML comment markers to activate
## Can Download Personal Data
Choose Yes or No

...or delete this placeholder if not applicable to this system
-->

<!-- Placeholder - remove HTML comment markers to activate
## Can Contact Individuals
Choose Yes or No

...or delete this placeholder if not applicable to this system
-->

## Failover Architecture Type

ActiveActive

## Failover Process Type

FullyAutomated

## Failback Process Type

FullyAutomated

## Failover Details

The service is deployed in both PAC clusters. The failover guide for the cluster is located [here](https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/pac-cluster).

## Data Recovery Process Type

NotApplicable

## Data Recovery Details

The service does not store data, so it does not require any data recovery steps.

## Release Process Type

PartiallyAutomated

## Rollback Process Type

Manual

## Release Details

Manual failover is needed when a new version of the service is deployed to production. Otherwise, an automated failover is going to take place when releasing.
For more details about the failover process see the [PAC failover guide](https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/pac-cluster).

<!-- Placeholder - remove HTML comment markers to activate
## Heroku Pipeline Name
Enter descriptive text satisfying the following:
This is the name of the Heroku pipeline for this system. If you don't have a pipeline, this is the name of the app in Heroku. A pipeline is a group of Heroku apps that share the same codebase where each app in a pipeline represents the different stages in a continuous delivery workflow, i.e. staging, production.

...or delete this placeholder if not applicable to this system
-->

## Key Management Process Type

Manual

## Key Management Details

The service uses sealed secrets to manage Kubernetes secrets.
The actions required to create/change sealed secrets are described [here](https://github.com/Financial-Times/upp-docs/tree/master/guides/sealed-secrets-guide/).

## Monitoring

Health Checks:

*   [PAC Prod EU](https://pac-prod-eu.upp.ft.com/__health/__pods-health?service-name=draft-annotations-api)
*   [PAC Prod US](https://pac-prod-us.upp.ft.com/__health/__pods-health?service-name=draft-annotations-api)

Splunk Alerts:

*   [Concepts failed to be enriched by draft annotations API](https://financialtimes.splunkcloud.com/en-US/app/financial_times_production/alert?s=%2FservicesNS%2Fnobody%2Ffinancial_times_production%2Fsaved%2Fsearches%2FPAC%2520Concepts%2520failed%2520to%2520be%2520enriched%2520by%2520draft%2520annotations%2520API)

## First Line Troubleshooting

Please refer to the [First Line Troubleshooting guide](https://github.com/Financial-Times/upp-docs/tree/master/guides/ops/first-line-troubleshooting).

## Second Line Troubleshooting

Please refer to the GitHub repository README for troubleshooting information.