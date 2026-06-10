module.exports = {
  apps: [
    {
      name: "flaresolverr",
      script: "/opt/flaresolverr/flaresolverr",
      cwd: "/opt/flaresolverr",
      watch: false,
      autorestart: true,
      restart_delay: 5000,
      max_restarts: 10,
      wait_ready: false,
      env: {
        LOG_LEVEL: "info",
        LOG_HTML: "false",
        HEADLESS: "true",
        PORT: "8191",
        HOST: "127.0.0.1",
      },
    },
    {
      name: "solscan-watcher",
      script: "./solscan-watcher", // compiled binary, see Makefile
      cwd: "/opt/solscan-watcher", // adjust to your actual path
      watch: false,
      autorestart: true,
      restart_delay: 5000,
      max_restarts: 10,
    },
  ],
};
 