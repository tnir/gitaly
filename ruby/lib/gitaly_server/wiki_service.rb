module GitalyServer
  class WikiService < Gitaly::WikiService::Service
    include Utils

    def wiki_delete_page(request, call)
      bridge_exceptions do
        repo = Gitlab::Git::Repository.from_call(call)
        wiki = Gitlab::Git::Wiki.new(repo)
        page_path = request.page_path
        commit_details = commit_details_from_gitaly(request.commit_details)

        wiki.delete_page(page_path, commit_details)

        Gitaly::WikiDeletePageResponse.new
      end
    end

    def wiki_write_page(call)
      bridge_exceptions do
        begin
          repo = Gitlab::Git::Repository.from_call(call)
          name = format = commit_details = nil
          content = ""

          call.each_remote_read.with_index do |request, index|
            if index.zero?
              name = request.name
              format = request.format
              commit_details = request.commit_details
            end

            content << request.content
          end

          wiki = Gitlab::Git::Wiki.new(repo)
          commit_details = commit_details_from_gitaly(commit_details)

          wiki.write_page(name, format.to_sym, content, commit_details)

          Gitaly::WikiWritePageResponse.new
        rescue Gitlab::Git::Wiki::DuplicatePageError => e
          Gitaly::WikiWritePageResponse.new(duplicate_error: e.message.b)
        end
      end
    end

    def wiki_find_page(request, call)
      bridge_exceptions do
        repo = Gitlab::Git::Repository.from_call(call)
        wiki = Gitlab::Git::Wiki.new(repo)

        page = wiki.page(
          title: request.title,
          version: request.revision.presence,
          dir: request.directory.presence
        )

        unless page
          return Enumerator.new do |y|
            y.yield Gitaly::WikiFindPageResponse.new
          end
        end

        version = Gitaly::WikiPageVersion.new(
          commit: gitaly_commit_from_rugged(page.version.commit.raw_commit),
          format: page.version.format.to_s
        )
        gitaly_wiki_page = Gitaly::WikiPage.new(
          version: version,
          format: page.format.to_s,
          title: page.title.b,
          url_path: page.url_path.to_s,
          path: page.path.b,
          name: page.name.b,
          historical: page.historical?
        )

        Enumerator.new do |y|
          io = StringIO.new(page.text_data)
          while chunk = io.read(Gitlab.config.git.write_buffer_size)
            gitaly_wiki_page.raw_data = chunk

            y.yield Gitaly::WikiFindPageResponse.new(page: gitaly_wiki_page)

            gitaly_wiki_page = Gitaly::WikiPage.new
          end
        end
      end
    end

    def wiki_find_file(request, call)
      bridge_exceptions do
        repo = Gitlab::Git::Repository.from_call(call)
        wiki = Gitlab::Git::Wiki.new(repo)

        file = wiki.file(request.name, request.revision.presence)

        unless file
          return Enumerator.new do |y|
            y.yield Gitaly::WikiFindFileResponse.new
          end
        end

        response = Gitaly::WikiFindFileResponse.new(
          name: file.name.b,
          mime_type: file.mime_type,
          path: file.path
        )

        Enumerator.new do |y|
          io = StringIO.new(file.raw_data)
          while chunk = io.read(Gitlab.config.git.write_buffer_size)
            response.raw_data = chunk

            y.yield response

            response = Gitaly::WikiFindFileResponse.new
          end
        end
      end
    end

    def wiki_get_page_versions(request, call)
      bridge_exceptions do
        repo = Gitlab::Git::Repository.from_call(call)
        wiki = Gollum::Wiki.new(repo.path)
        path = request.page_path

        page = wiki.paged(Gollum::Page.canonicalize_filename(path), File.split(path).first)

        unless page
          return Enumerator.new do |y|
            y.yield Gitaly::WikiGetPageVersionsResponse.new(versions: [])
          end
        end

        Enumerator.new do |y|
          page.versions.each_slice(20) do |slice|
            versions =
              slice.map do |commit|
                gollum_page = wiki.page(page.title, commit.id)
                obj = repo.rugged.rev_parse(commit.id)

                Gitaly::WikiPageVersion.new(
                  commit: gitaly_commit_from_rugged(obj),
                  format: gollum_page&.format.to_s
                )
              end

            y.yield Gitaly::WikiGetPageVersionsResponse.new(versions: versions)
          end
        end
      end
    end

    def wiki_update_page(call)
      bridge_exceptions do
        repo = Gitlab::Git::Repository.from_call(call)
        title = format = page_path = commit_details = nil
        content = ""

        wiki = Gitlab::Git::Wiki.new(repo)
        call.each_remote_read.with_index do |request, index|
          if index.zero?
            title = request.title
            page_path = request.page_path
            format = request.format

            commit_details = commit_details_from_gitaly(request.commit_details)
          end

          content << request.content
        end

        wiki.update_page(page_path, title, format.to_sym, content, commit_details)

        Gitaly::WikiUpdatePageResponse.new
      end
    end
  end
end
