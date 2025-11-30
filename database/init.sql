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

-- Insert my channel
insert into nivek.users (username, email)
values ('timallenfanclubofficial', 'timallenfanclubofficial@nivek.com')
ON CONFLICT (username) DO NOTHING;

-- Table to track fishing scores per user
create TABLE IF NOT EXISTS nivek.fish_score (
    id SERIAL PRIMARY KEY,
    channelname VARCHAT(255) NOT NULL,
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

-- Optional: Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_fish_score_user_chatter
ON nivek.fish_score(user_id, chattername);

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
