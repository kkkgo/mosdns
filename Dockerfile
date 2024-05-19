FROM alpine:edge AS check
RUN apk update && apk upgrade && apk add --no-cache go curl socat
WORKDIR /src
COPY . /src/
RUN rm go.mod go.sum
RUN go mod init github.com/IrineSistiana/mosdns/v5
RUN go get -u
RUN go build -ldflags "-s -w" -trimpath -o /usr/bin/mosdns
ADD https://raw.githubusercontent.com/kkkgo/Country-only-cn-private.mmdb/main/Country-only-cn-private.mmdb /src/
RUN sh /src/test.sh

FROM check
WORKDIR /src
CMD rm go.mod go.sum && go mod init github.com/IrineSistiana/mosdns/v5 && go get -u