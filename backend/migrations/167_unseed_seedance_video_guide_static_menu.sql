-- Remove the release-seeded Seedace guide menu item so operators can configure
-- the guide entry from System Settings instead of receiving a hard-coded URL.
-- The static HTML file remains available at /seedance-video-guide.html.

DO $$
DECLARE
    v_items jsonb := '[]'::jsonb;
    v_next jsonb := '[]'::jsonb;
    v_elem jsonb;
BEGIN
    SELECT COALESCE(value::jsonb, '[]'::jsonb)
      INTO v_items
      FROM settings
     WHERE key = 'custom_menu_items';

    IF v_items IS NULL THEN
        v_items := '[]'::jsonb;
    END IF;

    IF jsonb_typeof(v_items) <> 'array' THEN
        RETURN;
    END IF;

    FOR v_elem IN SELECT value FROM jsonb_array_elements(v_items)
    LOOP
        IF v_elem ->> 'id' = 'seedance_video_guide'
           AND v_elem ->> 'url' = 'https://cc-ai.xyz/seedance-video-guide.html'
           AND COALESCE(v_elem ->> 'content_md', '') = ''
           AND COALESCE(v_elem ->> 'page_slug', '') = '' THEN
            CONTINUE;
        END IF;
        v_next := v_next || jsonb_build_array(v_elem);
    END LOOP;

    UPDATE settings
       SET value = v_next::text
     WHERE key = 'custom_menu_items';
END $$;
