FROM python:3.14.0a3-alpine3.20

ENV API_KEY ""
ENV PYTHONIOENCODING=utf-8
ENV LANG=C.UTF-8
ENV ALLOWED_IDS ""
LABEL authors="tomer.klein@gmail.com"

RUN apt -yqq update && \
    apt -yqq install gnupg2 && \
    apt -yqq install curl unzip && \
    rm -rf /var/lib/apt/lists/*
    
RUN pip3 install --upgrade pip --no-cache-dir && \
    pip3 install --upgrade setuptools --no-cache-dir

COPY requirements.txt /tmp

RUN pip3 install -r /tmp/requirements.txt

RUN mkdir -p /app/db

COPY app /app

WORKDIR /opt/botami

CMD python app.py
