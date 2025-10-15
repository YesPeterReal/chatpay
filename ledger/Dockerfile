FROM golang:1.20

WORKDIR /app

# Copy dependency files first
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the app
RUN go build -o chatpay .

EXPOSE 8080
CMD ["./chatpay"]