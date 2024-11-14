FROM public.ecr.aws/docker/library/golang:1.20 as build
WORKDIR /lambda

COPY go.* *.go .
RUN go mod download
COPY ./pkg/ ./pkg/
RUN find . -print
RUN go build -tags lambda.norpc -o bootstrap .
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap -tags lambda.norpc .


FROM --platform=linux/amd64 public.ecr.aws/lambda/provided:al2023
COPY --from=build /lambda/bootstrap /usr/local/bin/
ENTRYPOINT [ "/usr/local/bin/bootstrap" ]
