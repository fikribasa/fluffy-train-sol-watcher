module.exports = {
  apps: [
    {
      name: 'solscan-watcher',
      script: './solscan-watcher',
      cwd: '/home/user/agents/projects/solscan-watcher',
      // .env is loaded by the binary itself via godotenv
      env: {
        NODE_ENV: 'production',
      },
      // JSON-structured logging from the binary — pm2 passes through cleanly
      out_file: '/home/user/agents/projects/solscan-watcher/logs/out.log',
      error_file: '/home/user/agents/projects/solscan-watcher/logs/err.log',
      log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
      merge_logs: true,
      // Restart on crash, but don't flood
      max_restarts: 5,
      min_uptime: '10s',
      restart_delay: 5000,
      // Signal handling — binary listens for SIGINT/SIGTERM
      kill_timeout: 10000,
      // Run as a daemon
      exec_mode: 'fork',
      instances: 1,
    },
  ],
}
