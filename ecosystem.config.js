module.exports = {
  apps: [
    {
      name: "solscan-watcher",
      script: "./solscan-watcher", // compiled binary, see Makefile
      cwd: "/home/user/agents/projects/solscan-watcher",
      watch: false,
      autorestart: true,
      restart_delay: 5000,
      max_restarts: 10,
    },
  ],
};
