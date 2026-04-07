FROM golang:1.25 AS builder
WORKDIR /app
COPY app/go.mod app/go.sum ./
RUN go mod download
COPY app/main.go ./
RUN CGO_ENABLED=0 go build -o /bin/http-server ./main.go

# Stage 1 - Create the final image just with the executable or necessary files to only run the application
FROM scratch
USER 10001:10001
ENV USER=10001
ENV GROUP=10001
COPY --from=builder /bin/http-server /bin/http-server
CMD ["/bin/http-server"]
