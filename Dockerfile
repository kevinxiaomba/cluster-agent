FROM golang

COPY "$PWD"/auth/kubeconfig /usr/local/auth/kubeconfig
COPY ./appdynamics /usr/local/go/src/github.com/sjeltuhin/clusterAgent/vendor/appdynamics
COPY "$PWD" /usr/local/go/src/github.com/sjeltuhin/clusterAgent

WORKDIR /usr/local/go/src/github.com/sjeltuhin/clusterAgent

RUN go get ./

RUN  go build

ENV ACCOUNT_NAME customer1
ENV ACCESS_KEY 45ed60c0-e6f4-4330-b0c2-9939a6884726
ENV CONTROLLER_URL 451controllerbase-oscert-aoaz7jqm.srv.ravcloud.com
ENV CONTROLLER_PORT 8090
ENV APPLICATION_NAME k8s
ENV TIER_NAME ClusterAgent
ENV NODE_NAME $TIER_NAME_Node1
ENV KUBE_CONFIG_PATH /usr/local/auth/kubeconfig
ENV REST_API_CREDENTIALS k8sresty@customer1:k8sresty
ENV EVENTS_API_URL http://451controllerbase-oscert-aoaz7jqm.srv.ravcloud.com:9080

CMD ./clusterAgent
