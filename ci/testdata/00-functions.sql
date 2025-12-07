-- Custom UUID functions for MariaDB compatibility
-- These functions provide UUID_TO_BIN and BIN_TO_UUID functionality

DELIMITER $$

-- Convert UUID string to binary(16)
-- Supports both standard UUID format and UUIDv7
CREATE FUNCTION IF NOT EXISTS UUID_TO_BIN(uuid CHAR(36))
RETURNS BINARY(16)
DETERMINISTIC
SQL SECURITY INVOKER
BEGIN
    RETURN UNHEX(CONCAT(
        SUBSTR(uuid, 1, 8),
        SUBSTR(uuid, 10, 4),
        SUBSTR(uuid, 15, 4),
        SUBSTR(uuid, 20, 4),
        SUBSTR(uuid, 25, 12)
    ));
END$$

-- Convert binary(16) to UUID string
CREATE FUNCTION IF NOT EXISTS BIN_TO_UUID(bin BINARY(16))
RETURNS CHAR(36)
DETERMINISTIC
SQL SECURITY INVOKER
BEGIN
    DECLARE hex_str CHAR(32);
    SET hex_str = HEX(bin);
    RETURN LOWER(CONCAT(
        SUBSTR(hex_str, 1, 8), '-',
        SUBSTR(hex_str, 9, 4), '-',
        SUBSTR(hex_str, 13, 4), '-',
        SUBSTR(hex_str, 17, 4), '-',
        SUBSTR(hex_str, 21, 12)
    ));
END$$

-- Generate UUIDv7 (time-ordered UUID)
-- Format: 48 bits timestamp + 12 bits random + 2 bits version + 62 bits random
CREATE FUNCTION IF NOT EXISTS UUID_V7()
RETURNS CHAR(36)
DETERMINISTIC
SQL SECURITY INVOKER
BEGIN
    DECLARE timestamp_ms BIGINT;
    DECLARE rand_a CHAR(3);
    DECLARE rand_b CHAR(16);
    DECLARE uuid_hex CHAR(32);

    -- Get current timestamp in milliseconds since epoch
    SET timestamp_ms = UNIX_TIMESTAMP(NOW(3)) * 1000 + MICROSECOND(NOW(3)) DIV 1000;

    -- Generate random parts
    SET rand_a = LPAD(HEX(FLOOR(RAND() * 4095)), 3, '0');
    SET rand_b = LPAD(HEX(FLOOR(RAND() * 281474976710655)), 16, '0');

    -- Construct UUIDv7: timestamp(48) + rand_a(12) + version(4) + rand_b(62)
    SET uuid_hex = CONCAT(
        LPAD(HEX(timestamp_ms), 12, '0'),
        rand_a,
        '7',  -- Version 7
        SUBSTR(rand_b, 1, 3),
        HEX(CONV(CONV(SUBSTR(rand_b, 4, 1), 16, 10) & 0x3 | 0x8, 10, 16)),  -- Variant bits
        SUBSTR(rand_b, 5, 12)
    );

    -- Format as UUID string
    RETURN LOWER(CONCAT(
        SUBSTR(uuid_hex, 1, 8), '-',
        SUBSTR(uuid_hex, 9, 4), '-',
        SUBSTR(uuid_hex, 13, 4), '-',
        SUBSTR(uuid_hex, 17, 4), '-',
        SUBSTR(uuid_hex, 21, 12)
    ));
END$$

DELIMITER ;
