FROM golang as builder

COPY ./appdynamics /usr/local/go/src/github.com/sjeltuhin/clusterAgent/vendor/appdynamics
COPY "$PWD" /usr/local/go/src/github.com/sjeltuhin/clusterAgent

WORKDIR /usr/local/go/src/github.com/sjeltuhin/clusterAgent

RUN go get ./

RUN GOOS=linux go build 

FROM centos:latest

RUN yum install -y bash && \
    yum clean all && \
    rm -rf /var/cache/yum

COPY --from=builder /usr/local/go/src/github.com/sjeltuhin/clusterAgent/clusterAgent /opt/appdynamics/clusterAgent
COPY --from=builder /usr/local/go/src/github.com/sjeltuhin/clusterAgent/templates/*.json /opt/appdynamics/templates/
COPY --from=builder /usr/local/go/src/github.com/sjeltuhin/clusterAgent/vendor/appdynamics/lib/libappdynamics.so /opt/appdynamics/lib/libappdynamics.so

RUN mkdir -p /opt/appdynamics/templates/deploy
RUN mkdir -p /opt/appdynamics/ssl

ENV LD_LIBRARY_PATH  /opt/appdynamics/lib/

RUN chgrp -R 0 /opt/appdynamics/ && \
    chmod -R g=u /opt/appdynamics/ 
	
EXPOSE 8989

CMD /opt/appdynamics/clusterAgent