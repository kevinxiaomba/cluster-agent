FROM golang as builder

COPY ./appdynamics /usr/local/go/src/github.com/sjeltuhin/clusterAgent/vendor/appdynamics
COPY "$PWD" /usr/local/go/src/github.com/sjeltuhin/clusterAgent

WORKDIR /usr/local/go/src/github.com/sjeltuhin/clusterAgent

RUN go get ./

RUN GOOS=linux go build -o /usr/local/bin/appdynamics

FROM alpine:latest

RUN apk add --no-cache bash

COPY --from=builder /usr/local/bin/appdynamics/clusterAgent /usr/local/bin/appdynamics/clusterAgent
COPY --from=builder /usr/local/go/src/github.com/sjeltuhin/clusterAgent/vendor/appdynamics/lib/libappdynamics.so /usr/local/lib/

ENV LD_LIBRARY_PATH  /usr/local/lib/

RUN chgrp -R 0 /opt && \
    chmod -R g=u /opt
	
EXPOSE 8989

CMD /usr/local/bin/appdynamics/clusterAgent