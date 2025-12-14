DELIMITER $$

-- Converts UUID string to BINARY(16)
CREATE OR REPLACE FUNCTION UUID_TO_BIN(uuid CHAR(36))
RETURNS BINARY(16)
DETERMINISTIC
SQL SECURITY INVOKER
BEGIN
    -- Input validation: ensure the string is the correct length (36 characters for hyphenated UUID)
    IF LENGTH(uuid) <> 36 THEN
        RETURN NULL;
    END IF;

    -- Strip hyphens using REPLACE() and convert the 32-char hex string to BINARY(16)
    RETURN UNHEX(REPLACE(uuid, '-', ''));
END$$

-- Converts BINARY(16) to UUID string
-- This is the inverse function, converting the binary back to a hyphenated UUID string.
CREATE OR REPLACE FUNCTION BIN_TO_UUID(bin BINARY(16))
RETURNS CHAR(36)
DETERMINISTIC
SQL SECURITY INVOKER
BEGIN
    DECLARE hex_str CHAR(32);
    SET hex_str = HEX(bin);

    -- Format the 32-char hex string with hyphens and convert to lowercase
    RETURN LOWER(CONCAT(
        SUBSTR(hex_str, 1, 8), '-',
        SUBSTR(hex_str, 9, 4), '-',
        SUBSTR(hex_str, 13, 4), '-',
        SUBSTR(hex_str, 17, 4), '-',
        SUBSTR(hex_str, 21, 12)
    ));
END$$

DELIMITER ;
