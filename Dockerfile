FROM golang:1.12 as builder

RUN mkdir -p $GOPATH/src/github.com/sergiorua/kube-tagger
ADD . $GOPATH/src/github.com/sergiorua/kube-tagger
WORKDIR $GOPATH/src/github.com/sergiorua/kube-tagger

RUN go get -d -v ./... && go install -v ./...

RUN go build main.go && ls -l && echo $GOPATH

FROM golang:1.12
RUN mkdir /app
COPY --from=builder /go/src/github.com/sergiorua/kube-tagger/main /app/
WORKDIR /app
CMD ["./main"]
