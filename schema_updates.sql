ALTER TABLE payment_requests DROP CONSTRAINT IF EXISTS payment_requests_pkey;
ALTER TABLE payment_requests ALTER COLUMN id DROP DEFAULT;
DROP SEQUENCE IF EXISTS payment_requests_id_seq;
ALTER TABLE payment_requests ALTER COLUMN id TYPE uuid USING (gen_random_uuid());
ALTER TABLE payment_requests ADD CONSTRAINT payment_requests_pkey PRIMARY KEY (id);