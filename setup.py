import os
from setuptools import setup, find_packages

# Utility function to read the README file.
# Used for the long_description.  It's nice, because now 1) we have a top level
# README file and 2) it's easier to type in the README file than to put a raw
# string in below ...
def read(fname):
    return open(os.path.join(os.path.dirname(__file__), fname)).read()

setup(
    # Includes all other files that are within your project folder
    include_package_data=True,

    # Name of your Package
    name='radiobuenavia',
    # Project Version
    version='1.0',

    # Description of your Package
    description='automate the download, mixing and upload of tracks',

    # Website for your Project or Github repo
    url="https://github.com/finn-ball/radiobuenavia",

    # Name of the Creator
    author='Finn Ball',

    # Creator's mail address
    author_email='finn.ball@codificasolutions.com',

    # Projects you want to include in your Package
    packages=find_packages(),

    # Dependencies/Other modules required for your package to work
    install_requires=['dropbox', 'toml'],

    # Detailed description of your package
    long_description=read("README.md"),

    # Format of your Detailed Description
    long_description_content_type="text/markdown",

    entry_points={"console_scripts": ["rbv = radiobuenavia.cli:cli"]},
)
