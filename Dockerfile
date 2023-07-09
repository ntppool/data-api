FROM alpine:3.18.0

RUN apk --no-cache upgrade
RUN apk --no-cache add ca-certificates tzdata zsh jq tmux curl

RUN addgroup -g 1000 app && adduser -u 1000 -D -G app app
RUN touch ~app/.zshrc ~root/.zshrc; chown app:app ~app/.zshrc

RUN mkdir /app
ADD dist/data-api_linux_amd64_v1/data-api /app/

EXPOSE 4200
EXPOSE 4290

USER app

# Container start command for production
CMD ["/app/data-api"]