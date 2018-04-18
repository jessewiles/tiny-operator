docker-build:
	@eval $$(minikube docker-env); \
	docker build -t local-echo-server:0.1 .
