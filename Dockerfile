FROM golang:1

ENV PROJECT=draft-annotations-api

ENV ORG_PATH="github.com/Financial-Times"
ENV SRC_FOLDER="${GOPATH}/src/${ORG_PATH}/${PROJECT}"

COPY . ${SRC_FOLDER}
WORKDIR ${SRC_FOLDER}

ARG GITHUB_USERNAME
ARG GITHUB_TOKEN

RUN BUILDINFO_PACKAGE="${ORG_PATH}/service-status-go/buildinfo." \
  && VERSION="version=$(git describe --tag --always 2> /dev/null)" \
  && DATETIME="dateTime=$(date -u +%Y%m%d%H%M%S)" \
  && REPOSITORY="repository=$(git config --get remote.origin.url)" \
  && REVISION="revision=$(git rev-parse HEAD)" \
  && BUILDER="builder=$(go version)" \
  && if [ ! -z ${GITHUB_USERNAME} ] && [ ! -z ${GITHUB_TOKEN} ]; then \
    git config --global url."https://${GITHUB_USERNAME}:${GITHUB_TOKEN}@github.com".insteadOf "https://github.com" ; \
    fi \
  && LDFLAGS="-X '"${BUILDINFO_PACKAGE}$VERSION"' -X '"${BUILDINFO_PACKAGE}$DATETIME"' -X '"${BUILDINFO_PACKAGE}$REPOSITORY"' -X '"${BUILDINFO_PACKAGE}$REVISION"' -X '"${BUILDINFO_PACKAGE}$BUILDER"'" \
  && echo "Build flags: $LDFLAGS" \
  && CGO_ENABLED=0 GO111MODULE=on go build -mod=readonly -o /artifacts/${PROJECT} -v -ldflags="${LDFLAGS}" 
  

FROM scratch
WORKDIR /
COPY ./_ft/api.yml /_ft/
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=0 /artifacts/* /

CMD [ "/draft-annotations-api" ]
