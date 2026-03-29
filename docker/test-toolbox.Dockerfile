FROM golang:1.26-alpine

RUN apk add --no-cache \
  bash \
  bind-tools \
  curl \
  git \
  jq \
  make \
  netcat-openbsd \
  nodejs \
  npm \
  postgresql17-client \
  python3 \
  py3-pip

WORKDIR /workspace
