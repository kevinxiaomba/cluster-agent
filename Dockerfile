FROM golang

COPY "$PWD"/auth/kubeconfig /usr/local/auth/kubeconfig


WORKDIR /usr/local/go/src/github.com/sjeltuhin/clusterAgent

RUN go get ./

RUN  go build


CMD ./clusterAgent
