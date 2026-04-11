CONTAINER_NAME=llt-trade
HEALTH_CHECK_PORT=8080
METRICS_PORT=4014

.PHONY: docker docker-build docker-run docker-stop docker-rm docker-restart docker-upgrade build run stop rm restart upgrade

docker:
	@action=$(word 2,$(MAKECMDGOALS)); \
	if [ -z "$$action" ]; then \
		echo "Usage: make docker {build|run|stop|rm|restart|upgrade}"; \
		exit 1; \
	fi; \
	case "$$action" in \
		build) $(MAKE) docker-build ;; \
		run) $(MAKE) docker-run ;; \
		stop) $(MAKE) docker-stop ;; \
		rm) $(MAKE) docker-rm ;; \
		restart) $(MAKE) docker-restart ;; \
		upgrade) $(MAKE) docker-upgrade ;; \
		*) echo "Unknown docker action: $$action"; exit 1 ;; \
	esac

# 占位目标，配合 "make docker build|run|..." 使用（避免 Make 把第二个词当成独立目标）
build run stop rm restart upgrade:
	@:

docker-build:
	@echo "--> Building docker image (context: repo root)"
	@DOCKER_BUILDKIT=1 docker build --platform=linux/amd64 \
		-f ./Dockerfile \
		-t ${CONTAINER_NAME} .

docker-run:
	@echo "--> Running docker container"
	@docker run \
		-d \
		--restart unless-stopped \
		-p 8000:8000 -p 3000:3000 -p $(HEALTH_CHECK_PORT):8080 -p $(METRICS_PORT):4014 \
		--health-cmd="curl -f http://localhost:$(HEALTH_CHECK_PORT)/health/alive || exit 1" \
		--health-interval=60s \
		--health-start-period=3s \
		--env-file ./secrets/.docker.env \
		-v /etc/localtime:/etc/localtime:ro \
        -v /etc/timezone:/etc/timezone:ro \
		--network alva \
		--name ${CONTAINER_NAME} \
		${CONTAINER_NAME}:latest

docker-stop:
	@echo "--> Stopping docker container"
	@docker container stop ${CONTAINER_NAME} || true

docker-rm: docker-stop
	@echo "--> Removing docker container"
	@docker container rm -f ${CONTAINER_NAME} || true

docker-restart: docker-rm docker-run

docker-upgrade: docker-build docker-restart
