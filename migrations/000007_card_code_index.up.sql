-- Tra card don theo ma (header.code, vd WEB-20261123-0001) cho
-- GET /api/client/v1/chat/cards/:code — khong quet ca bang messages.
CREATE INDEX IF NOT EXISTS ix_messages_card_code
    ON messages ((card -> 'header' ->> 'code'), created_at DESC)
    WHERE card IS NOT NULL;
