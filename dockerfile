FROM golang:onbuild
EXPOSE 8000
RUN go get -v github.com/karthikraobr/ta-go
#docker build -t karthikraobr/numbers .
#docker run -p 8000:8000 -t karthikraobr/numbers