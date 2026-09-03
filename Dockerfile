FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/faulthub ./cmd/faulthub \
	&& mkdir -p /out/data && chown 65534:65534 /out/data

FROM scratch
COPY --from=build /out/faulthub /faulthub
COPY --from=build --chown=65534:65534 /out/data /data
USER 65534:65534
VOLUME /data
ENV FAULTHUB_DATA_DIR=/data
# Documentary only: FAULTHUB_PORT remaps the actual listen/publish ports at runtime.
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
	CMD ["/faulthub", "healthcheck"]
ENTRYPOINT ["/faulthub"]