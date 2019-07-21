FROM golang as builder

COPY ./appdynamics /usr/local/go/src/github.com/appdynamics/cluster-agent/vendor/appdynamics
COPY "$PWD" /usr/local/go/src/github.com/appdynamics/cluster-agent

WORKDIR /usr/local/go/src/github.com/appdynamics/cluster-agent

RUN go get ./

RUN GOOS=linux go build 

FROM centos:latest

MAINTAINER AppDynamics

LABEL name="AppDynamics ClusterAgent" \
      vendor="AppDynamics" \
      version="0.2.6" \
      release="1" \
      url="https://www.appdynamics.com" \
      summary="AppDynamics monitoring solution for applications deployed to Kubernetes clusters" \
      description="The ClusterAgent monitors state of Kuberenetes resources and derives metrics to provide visibility into common application impacting issues."

RUN yum install -y bash && \
    yum clean all && \
    rm -rf /var/cache/yum

COPY --from=builder /usr/local/go/src/github.com/appdynamics/cluster-agent/cluster-agent /opt/appdynamics/cluster-agent
COPY --from=builder /usr/local/go/src/github.com/appdynamics/cluster-agent/build/appdsaascert.pem /opt/appdynamics/ssl/appdsaascert.pem
COPY --from=builder /usr/local/go/src/github.com/appdynamics/cluster-agent/templates/*.json /opt/appdynamics/templates/
COPY --from=builder /usr/local/go/src/github.com/appdynamics/cluster-agent/vendor/appdynamics/lib/libappdynamics.so /opt/appdynamics/lib/libappdynamics.so

COPY --from=builder /usr/local/go/src/github.com/appdynamics/cluster-agent/LICENSE /licenses/

RUN mkdir -p /opt/appdynamics/templates/deploy

ENV LD_LIBRARY_PATH  /opt/appdynamics/lib/


RUN chgrp -R 0 /opt/appdynamics/ && \
    chmod -R g=u /opt/appdynamics/ 
	
EXPOSE 8989

CMD /opt/appdynamics/cluster-agent