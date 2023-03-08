FROM golang:1.10-alpine as build

WORKDIR /go/src/github.com/YoYoContainerService/xpu-scheduler-extender
COPY . .

RUN go build -o /go/bin/xpu-scheduler-extender cmd/*.go

FROM alpine

COPY --from=build /go/bin/xpu-scheduler-extender /usr/bin/xpu-scheduler-extender

CMD ["xpu-scheduler-extender"]
