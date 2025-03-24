const cardValidator = require("simple-card-validator");
const { v4: uuidv4 } = require("uuid");
const pino = require("pino");
const pool = require("./db");

const logger = pino({
  name: "paymentservice-charge",
  messageKey: "message",
  formatters: {
    level(logLevelString, logLevelNum) {
      return { severity: logLevelString };
    },
  },
});

// Ensure table exists
async function ensureTableExists() {
  const tableName = process.env.POSTGRES_TABLE || "transactions";
  try {
    await pool.query(`
      CREATE TABLE IF NOT EXISTS ${tableName} (
        id UUID PRIMARY KEY,
        card_number TEXT NOT NULL,
        card_type TEXT NOT NULL,
        amount TEXT NOT NULL,
        currency TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT NOW()
      )
    `);
    logger.info(`Table ${tableName} checked/created successfully`);
  } catch (err) {
    logger.error(`Error ensuring table exists: ${err.message}`);
    throw new Error("Database setup failed");
  }
}

// Call table check on module load
ensureTableExists();

module.exports = async function charge(request) {
  const { amount, credit_card: creditCard } = request;
  const cardNumber = creditCard.credit_card_number;
  const cardInfo = cardValidator(cardNumber);
  const { card_type: cardType, valid } = cardInfo.getCardDetails();

  if (!valid) {
    throw new Error("Invalid Credit Card");
  }
  if (!(cardType === "visa" || cardType === "mastercard")) {
    throw new Error(`Unsupported card type: ${cardType}`);
  }

  const transaction_id = uuidv4();
  const { units, nanos, currency_code } = amount;
  const tableName = process.env.POSTGRES_TABLE || "transactions";

  try {
    await pool.query(
      `INSERT INTO ${tableName} (id, card_number, card_type, amount, currency) VALUES ($1, $2, $3, $4, $5)`,
      [
        transaction_id,
        cardNumber.slice(-4),
        cardType,
        `${units}.${nanos}`,
        currency_code,
      ],
    );
    logger.info(`Transaction ${transaction_id} processed successfully`);
  } catch (err) {
    logger.error(`Database error: ${err.message}`);
    throw new Error("Failed to process transaction");
  }

  return { transaction_id };
};
