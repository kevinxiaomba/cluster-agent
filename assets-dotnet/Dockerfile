
FROM openjdk:8-jdk-alpine

RUN apk add --no-cache grep bc coreutils

RUN mkdir -p /opt/temp

COPY AppDynamics.Agent.netstandard.dll /opt/temp/AppDynamics.Agent.netstandard.dll
COPY libappdprofiler.so /opt/temp/libappdprofiler.so

CMD ["tail", "-f", "/dev/null"]
