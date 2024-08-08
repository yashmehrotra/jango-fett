import yaml


def file_name(raw_contents):
    contents = yaml.safe_load(raw_contents)
    kind = contents['kind']
    metadata_name = contents['metadata']['name']
    return f'{kind}-{metadata_name}.yaml'

def write_to_file(raw_contents):
    name = file_name(raw_contents)
    f = open(name, 'w')
    f.write(raw_contents)
    f.close()


def main():
    main_file = input('Enter file path of complete yaml: ')
    yaml_content = open(main_file, 'r').read()
    yamls = yaml_content.split('---')

    for single_yaml in yamls:
        write_to_file(single_yaml)


main()
