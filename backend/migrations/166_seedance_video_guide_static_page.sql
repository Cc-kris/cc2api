-- Point the Seedance video guide custom menu item to the static HTML page.
--
-- Idempotent: updates the existing item with id "seedance_video_guide" when present;
-- otherwise inserts it immediately after the API key configuration guide when found.

DO $$
DECLARE
    v_raw            text;
    v_items          jsonb;
    v_next           jsonb := '[]'::jsonb;
    v_elem           jsonb;
    v_new_item       jsonb;
    v_inserted       boolean := false;
    v_found_existing boolean := false;
    v_sort_order     int := 0;
BEGIN
    SELECT value INTO v_raw
      FROM settings WHERE key = 'custom_menu_items';

    IF COALESCE(v_raw, '') = '' OR v_raw = 'null' THEN
        v_items := '[]'::jsonb;
    ELSE
        v_items := v_raw::jsonb;
    END IF;

    v_new_item := jsonb_build_object(
        'id',         'seedance_video_guide',
        'label',      'seedace视频调用说明',
        'icon_svg',   '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 6.75h9A2.25 2.25 0 0115.75 9v6A2.25 2.25 0 0113.5 17.25h-9A2.25 2.25 0 012.25 15V9A2.25 2.25 0 014.5 6.75z"/></svg>',
        'url',        'https://cc-ai.xyz/seedance-video-guide.html',
        'visibility', 'user',
        'sort_order', 0
    );

    FOR v_elem IN SELECT jsonb_array_elements(v_items) LOOP
        IF v_elem ->> 'id' = 'seedance_video_guide' THEN
            v_next := v_next || jsonb_build_array(
                v_new_item || jsonb_build_object(
                    'sort_order',
                    CASE
                        WHEN COALESCE(v_elem ->> 'sort_order', '') ~ '^[0-9]+$'
                        THEN (v_elem ->> 'sort_order')::int
                        ELSE v_sort_order
                    END
                )
            );
            v_found_existing := true;
        ELSE
            v_next := v_next || jsonb_build_array(
                jsonb_set(v_elem, '{sort_order}', to_jsonb(v_sort_order), true)
            );

            IF NOT v_inserted
               AND NOT v_found_existing
               AND (
                 v_elem ->> 'id' IN ('api_key_config_guide', 'api-key-config-guide', 'api_key_guide')
                 OR v_elem ->> 'label' IN ('API秘钥配置说明', 'API密钥配置说明', 'API 密钥配置说明', 'API Key 配置说明')
               ) THEN
                v_sort_order := v_sort_order + 1;
                v_next := v_next || jsonb_build_array(
                    v_new_item || jsonb_build_object('sort_order', v_sort_order)
                );
                v_inserted := true;
            END IF;
        END IF;
        v_sort_order := v_sort_order + 1;
    END LOOP;

    IF NOT v_found_existing AND NOT v_inserted THEN
        v_next := v_next || jsonb_build_array(
            v_new_item || jsonb_build_object('sort_order', v_sort_order)
        );
    END IF;

    INSERT INTO settings (key, value)
    VALUES ('custom_menu_items', v_next::text)
    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
END $$;
