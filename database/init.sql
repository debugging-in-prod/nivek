-- Create a schema for the project
create SCHEMA IF NOT EXISTS nivek;

-- Application-level table
-- contains information unique to the bot itself
create TABLE IF NOT EXISTS nivek.app (
    id SERIAL PRIMARY KEY,
    last_wiped_at TIMESTAMP
);

-- Create users table
-- this table will represent every channel the twitch bot should join. The "users" of the twitch bot
CREATE TABLE IF NOT EXISTS nivek.users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    email VARCHAR(100) NOT NULL UNIQUE, -- filler for now
    password TEXT NOT NULL,             -- added password field
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Table to track fishing scores per user
create TABLE IF NOT EXISTS nivek.fish_score (
    id SERIAL PRIMARY KEY,
    channelname VARCHAR(255) NOT NULL,
    chattername VARCHAR(255) NOT NULL,
    score INT NOT NULL DEFAULT 0,
    fish JSONB NOT NULL DEFAULT '[]',
    trash_caught INT NOT NULL DEFAULT 0,
    times_fished INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Composite unique constraint
    CONSTRAINT unique_user_chatter UNIQUE (channelname, chattername)
);

-- Table to track bread counts
create TABLE IF NOT EXISTS nivek.bread (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    chattername VARCHAR(255) NOT NULL,
    count INT DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Composite unique constraint
    CONSTRAINT unique_user_chatter UNIQUE (user_id, chattername)
);

-- Auto shoutout whitelist system
CREATE TABLE IF NOT EXISTS nivek.auto_shout (
    id SERIAL PRIMARY KEY,
    channelname VARCHAR(255) NOT NULL,
    chattername VARCHAR(255) NOT NULL,
    shout_count int NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS nivek.bread (
    id SERIAL PRIMARY KEY,
    channelname VARCHAR(255) NOT NULL,
    chattername VARCHAR(255) NOT NULL,
    bread_count int NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    CONSTRAINT bread_channel_chatter_unique
    UNIQUE (channelname, chattername)
);

CREATE TABLE lurk (
  id SERIAL PRIMARY KEY,
  channelname   TEXT NOT NULL,
  chattername   TEXT NOT NULL,
  lurk_count    INTEGER NOT NULL DEFAULT 0,
  created       TIMESTAMP NOT NULL DEFAULT NOW(),
  updated       TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Composite unique constraint
    CONSTRAINT unique_lurker UNIQUE (channelname, chattername)
);
