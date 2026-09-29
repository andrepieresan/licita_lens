FROM oven/bun:1.3.14

WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY app.json ./
COPY App.tsx tsconfig.json env.d.ts ./
COPY src ./src
RUN chown -R bun:bun /app

USER bun
ENV CI=1
EXPOSE 19006
CMD ["bun", "run", "start", "--", "--web", "--host", "lan", "--port", "19006"]
