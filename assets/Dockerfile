
FROM openjdk:8-jdk-alpine

RUN apk add --no-cache grep bc coreutils

RUN mkdir -p /opt/temp

COPY AppServerAgent/ /opt/temp
COPY tools.jar/ /opt/temp

CMD ["tail", "-f", "/dev/null"]
