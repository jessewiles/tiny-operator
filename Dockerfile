FROM ubuntu:18.04

RUN apt update && apt install -y python python-setuptools curl

COPY ./server.py /usr/share/server.py

ENV SOMEWHERE "OVER THE RAINBOW"

EXPOSE 8000
CMD ["/usr/bin/python", "/usr/share/server.py"]
