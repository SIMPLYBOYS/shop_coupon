-- Table to store user information
CREATE TABLE users (
    id SERIAL PRIMARY KEY, -- Auto-incrementing primary key for users
    username VARCHAR(50) NOT NULL, -- Username, must not be null
    email VARCHAR(50) NOT NULL UNIQUE, -- Email address, must be unique
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP -- Timestamp when the user was created
);

-- Table to store coupon information
CREATE TABLE coupons (
    id SERIAL PRIMARY KEY, -- Auto-incrementing primary key for coupons
    code VARCHAR(48) NOT NULL UNIQUE, -- Unique coupon code
    discount NUMERIC(5, 2) NOT NULL, -- Discount amount for the coupon
    expiry_date DATE NOT NULL, -- Expiry date for the coupon
    is_used BOOLEAN NOT NULL DEFAULT FALSE, -- Flag indicating if the coupon has been used
    user_id INT UNIQUE REFERENCES users(id), -- Foreign key referencing the user who claimed the coupon, must be unique
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, -- Timestamp when the coupon was created
    CONSTRAINT coupons_user_id_unique UNIQUE (user_id) -- Constraint to ensure user_id is unique
);

-- Table to store coupon reservation information
CREATE TABLE coupon_reservations (
    id SERIAL PRIMARY KEY, -- Auto-incrementing primary key for coupon reservations
    user_id INT NOT NULL, -- Foreign key referencing the user who made the reservation
    reserved_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, -- Timestamp when the reservation was made
    is_processed BOOLEAN DEFAULT FALSE, -- Flag indicating if the reservation has been processed
    FOREIGN KEY (user_id) REFERENCES users(id), -- Foreign key constraint referencing the users table
    UNIQUE (user_id) -- Constraint to ensure user_id is unique
);

-- Create a unique index on the email column of the users table
CREATE UNIQUE INDEX idx_users_email ON users (email);

-- Create a unique index on the code column of the coupons table
CREATE UNIQUE INDEX idx_coupons_code ON coupons (code);

-- Create a composite index on the user_id and created_at columns of the coupons table
CREATE INDEX idx_coupons_user_id_created_at ON coupons (user_id, created_at);

-- Create a composite index on the user_id and reserved_at columns of the coupon_reservations table
CREATE INDEX idx_coupon_reservations_user_id_reserved_at ON coupon_reservations (user_id, reserved_at);