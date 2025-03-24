const { Pool } = require("pg");

const pool = new Pool({
  host: process.env.POSTGRES_HOST,
  port: process.env.POSTGRES_PORT,
  user: process.env.POSTGRES_USER,
  password: process.env.POSTGRES_PASSWORD,
  database: process.env.POSTGRES_DB,
  max: 10, // Số lượng connection tối đa
  idleTimeoutMillis: 30000, // Timeout sau 30s nếu không dùng
});

pool.on("connect", () => {
  console.log("Connected to PostgreSQL RDS");
});

module.exports = pool;
