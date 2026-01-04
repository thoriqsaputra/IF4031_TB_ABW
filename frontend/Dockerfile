FROM node:20-alpine AS base
WORKDIR /app

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

FROM base AS dev
COPY frontend/ ./
ENV HOST=0.0.0.0
ENV PORT=3005
EXPOSE 3005
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "3005"]

FROM base AS build
COPY frontend/ ./
RUN npm run build

FROM node:20-alpine AS prod
WORKDIR /app
ENV NODE_ENV=production
ENV HOST=0.0.0.0
ENV PORT=3005
COPY --from=build /app/.output ./.output
EXPOSE 3005
CMD ["node", ".output/server/index.mjs"]
