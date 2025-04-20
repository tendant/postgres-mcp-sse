INSERT INTO users (id, email) VALUES (gen_random_uuid(), 'test@example.com');
INSERT INTO orders (id, user_id, amount) SELECT gen_random_uuid(), id, 99.99 FROM users LIMIT 1;