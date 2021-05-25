build:
	go build -o kube-tagger main.go

run:
	go run main.go -l

docker:
	docker build -t kube-tagger .
