FROM node:24-alpine AS builder
WORKDIR /app
COPY ./nivek-vue .
# Install dependencies — force legacy peer deps because Vite has peer issues on Node 24
RUN npm install --legacy-peer-deps --no-fund
# Expose the port
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host"]
