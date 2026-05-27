DO $$
BEGIN
    ALTER TABLE attachment_files DROP CONSTRAINT IF EXISTS attachment_files_kind_check;
    ALTER TABLE attachment_files ADD CONSTRAINT attachment_files_kind_check CHECK (kind IN ('image', 'video', 'audio', 'file'));
END $$;
