FROM golang:1.13 as builder

RUN mkdir -p $GOPATH/src/github.com/sergiorua/kube-tagger
ADD . $GOPATH/src/github.com/sergiorua/kube-tagger
WORKDIR $GOPATH/src/github.com/sergiorua/kube-tagger

RUN GO111MODULE=on go build main.go && ls -l && echo $GOPATH

FROM golang:1.13
RUN mkdir /app
COPY --from=builder /go/src/github.com/sergiorua/kube-tagger/main /app/
WORKDIR /app
CMD ["./main"]
