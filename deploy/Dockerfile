FROM golang

COPY ./appdynamics /usr/local/go/src/github.com/sjeltuhin/clusterAgent/vendor/appdynamics
COPY "$PWD" /usr/local/go/src/github.com/sjeltuhin/clusterAgent
COPY ./assets /usr/local/go/src/github.com/sjeltuhin/clusterAgent/assets
WORKDIR /usr/local/go/src/github.com/sjeltuhin/clusterAgent

RUN go get ./

RUN  go build

CMD ./clusterAgent