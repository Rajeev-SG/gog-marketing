DO $$
DECLARE
  connection_record RECORD;
  grant_entry RECORD;
  cleaned jsonb;
  target_tool text;
  target_key text;
  grant_value jsonb;
BEGIN
  FOR connection_record IN SELECT id, tool_grants_json FROM google_connections LOOP
    cleaned := '{}'::jsonb;

    IF jsonb_typeof(connection_record.tool_grants_json::jsonb) = 'object' THEN
      FOR grant_entry IN SELECT key, value FROM jsonb_each(connection_record.tool_grants_json::jsonb) LOOP
        CONTINUE WHEN grant_entry.value->>'tool' = '*';

        target_tool := NULL;
        IF grant_entry.value->>'service' = 'gmail' AND grant_entry.value->>'tool' = 'gmail_search' THEN
          target_tool := 'gmail_search';
        ELSIF grant_entry.value->>'service' = 'calendar' AND grant_entry.value->>'tool' = 'calendar_events' THEN
          target_tool := 'calendar_events';
        ELSIF grant_entry.value->>'service' = 'drive' AND grant_entry.value->>'tool' = 'drive_search' THEN
          target_tool := 'drive_search';
        END IF;

        IF target_tool IS NOT NULL THEN
          target_key := grant_entry.value->>'service' || '/' || target_tool;
          cleaned := cleaned || jsonb_build_object(target_key, grant_entry.value);
        END IF;
      END LOOP;

      FOR grant_entry IN SELECT key, value FROM jsonb_each(connection_record.tool_grants_json::jsonb) LOOP
        CONTINUE WHEN grant_entry.value->>'tool' <> '*';

        target_tool := NULL;
        IF grant_entry.value->>'service' = 'gmail' THEN
          target_tool := 'gmail_search';
        ELSIF grant_entry.value->>'service' = 'calendar' THEN
          target_tool := 'calendar_events';
        ELSIF grant_entry.value->>'service' = 'drive' THEN
          target_tool := 'drive_search';
        END IF;

        IF target_tool IS NOT NULL THEN
          target_key := grant_entry.value->>'service' || '/' || target_tool;
          IF NOT (cleaned ? target_key) THEN
            grant_value := jsonb_set(grant_entry.value, '{tool}', to_jsonb(target_tool));
            cleaned := cleaned || jsonb_build_object(target_key, grant_value);
          END IF;
        END IF;
      END LOOP;
    END IF;

    UPDATE google_connections
    SET tool_grants_json = cleaned::text
    WHERE id = connection_record.id;
  END LOOP;
END $$;
