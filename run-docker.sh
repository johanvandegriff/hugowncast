#!/bin/bash
docker build -t epjane/hugowncast:latest . && \
docker run --rm -p 8080:8080 -v ./data:/app/data -v ./hugo:/app/hugo epjane/hugowncast:latest
